// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/btree"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/storagepb"
)

const degree = 8

// unseqKey formats a key for use in a tree's BTree store.
// The associated Item value will be the stubtreeProto with the given nodeID
// prefix.
func subtreeKey(treeID, rev int64, nodeID storage.NodeID) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/subtree/%s/%d", treeID, nodeID.String(), rev)}
}

// tree stores all data for a given treeID
type tree struct {
	// mu protects access to all tree members.
	mu sync.RWMutex
	// store is a key-value representation of a Trillian tree storage.
	// The keyspace is partitioned off into various prefixes for the different
	// 'tables' of things stored in there.
	// e.g. subtree protos are stored with a key returned by subtreeKey() above.
	//
	// Other prefixes are used by Log/Map Storage.
	//
	// See the various key formatting functions for details of what is stored
	// under the formatted keys.
	//
	// store uses a BTree so that we can have a defined ordering over things
	// (such as sequenced leaves), while still accessing by key.
	store *btree.BTree
	// currentSTH is the timestamp of the current STH.
	currentSTH uint64
	meta       *trillian.Tree
}

func (t *tree) Lock() {
	t.mu.Lock()
}

func (t *tree) Unlock() {
	t.mu.Unlock()
}

func (t *tree) RLock() {
	t.mu.RLock()
}

func (t *tree) RUnlock() {
	t.mu.RUnlock()
}

// TreeStorage is shared between the memoryLog and (forthcoming) memoryMap-
// Storage implementations, and contains functionality which is common to both,
type TreeStorage struct {
	// mu only protects access to the trees map.
	mu    sync.RWMutex
	trees map[int64]*tree
}

// NewTreeStorage returns a new instance of the in-memory tree storage database.
func NewTreeStorage() *TreeStorage {
	return &TreeStorage{
		trees: make(map[int64]*tree),
	}
}

// getTree returns the tree associated with id, or nil if no such tree exists.
func (m *TreeStorage) getTree(id int64) *tree {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trees[id]
}

// kv is a simple key->value type which implements btree's Item interface.
type kv struct {
	k string
	v interface{}
}

// Less than by k's string key
func (a kv) Less(b btree.Item) bool {
	return strings.Compare(a.k, b.(*kv).k) < 0
}

// newTree creates and initializes a tree struct.
func newTree(t trillian.Tree) *tree {
	ret := &tree{
		store: btree.New(degree),
		meta:  &t,
	}
	k := unseqKey(t.TreeId)
	k.(*kv).v = list.New()
	ret.store.ReplaceOrInsert(k)

	k = hashToSeqKey(t.TreeId)
	k.(*kv).v = make(map[string][]int64)
	ret.store.ReplaceOrInsert(k)

	return ret
}

func (m *TreeStorage) beginTreeTX(ctx context.Context, treeID int64, hashSizeBytes int, cache cache.SubtreeCache, readonly bool) (treeTX, error) {
	tree := m.getTree(treeID)
	// Lock the tree for the duration of the TX.
	// It will be unlocked by a call to Commit or Rollback.
	var unlock func()
	if readonly {
		tree.RLock()
		unlock = tree.RUnlock
	} else {
		tree.Lock()
		unlock = tree.Unlock
	}
	return treeTX{
		ts:            m,
		tx:            tree.store.Clone(),
		tree:          tree,
		treeID:        treeID,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  cache,
		writeRevision: -1,
		unlock:        unlock,
	}, nil
}

type treeTX struct {
	closed        bool
	tx            *btree.BTree
	ts            *TreeStorage
	tree          *tree
	treeID        int64
	hashSizeBytes int
	subtreeCache  cache.SubtreeCache
	writeRevision int64
	unlock        func()
}

func (t *treeTX) getSubtree(ctx context.Context, treeRevision int64, nodeID storage.NodeID) (*storagepb.SubtreeProto, error) {
	s, err := t.getSubtrees(ctx, treeRevision, []storage.NodeID{nodeID})
	if err != nil {
		return nil, err
	}
	switch len(s) {
	case 0:
		return nil, nil
	case 1:
		return s[0], nil
	default:
		return nil, fmt.Errorf("got %d subtrees, but expected 1", len(s))
	}
}

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, nodeIDs []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(nodeIDs))

	for _, nodeID := range nodeIDs {
		if nodeID.PrefixLenBits%8 != 0 {
			return nil, fmt.Errorf("invalid subtree ID - not multiple of 8: %d", nodeID.PrefixLenBits)
		}

		// Look for a nodeID at or below treeRevision:
		for r := treeRevision; r >= 0; r-- {
			s := t.tx.Get(subtreeKey(t.treeID, r, nodeID))
			if s == nil {
				continue
			}
			// Return a copy of the proto to protect against the caller modifying the stored one.
			p := s.(*kv).v.(*storagepb.SubtreeProto)
			v := proto.Clone(p).(*storagepb.SubtreeProto)
			ret = append(ret, v)
			break
		}
	}

	// The InternalNodes cache is possibly nil here, but the SubtreeCache (which called
	// this method) will re-populate it.
	return ret, nil
}

func (t *treeTX) storeSubtrees(ctx context.Context, subtrees []*storagepb.SubtreeProto) error {
	if len(subtrees) == 0 {
		glog.Warning("attempted to store 0 subtrees...")
		return nil
	}

	for _, s := range subtrees {
		s := s
		if s.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", s))
		}
		k := subtreeKey(t.treeID, t.writeRevision, *storage.NewNodeIDFromHash(s.Prefix))
		k.(*kv).v = s
		t.tx.ReplaceOrInsert(k)
	}
	return nil
}

// getSubtreesAtRev returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesAtRev(ctx context.Context, rev int64) cache.GetSubtreesFunc {
	return func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, rev, ids)
	}
}

// GetMerkleNodes returns the requests nodes at (or below) the passed in treeRevision.
func (t *treeTX) GetMerkleNodes(ctx context.Context, treeRevision int64, nodeIDs []storage.NodeID) ([]storage.Node, error) {
	return t.subtreeCache.GetNodes(nodeIDs, t.getSubtreesAtRev(ctx, treeRevision))
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []storage.Node) error {
	for _, n := range nodes {
		err := t.subtreeCache.SetNodeHash(n.NodeID, n.Hash,
			func(nID storage.NodeID) (*storagepb.SubtreeProto, error) {
				return t.getSubtree(ctx, t.writeRevision, nID)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *treeTX) Commit() error {
	defer t.unlock()

	if t.writeRevision > -1 {
		if err := t.subtreeCache.Flush(func(st []*storagepb.SubtreeProto) error {
			return t.storeSubtrees(context.TODO(), st)
		}); err != nil {
			glog.Warningf("TX commit flush error: %v", err)
			return err
		}
	}
	t.closed = true
	// update the shared view of the tree post TX:
	t.tree.store = t.tx
	return nil
}

func (t *treeTX) Rollback() error {
	defer t.unlock()

	t.closed = true
	return nil
}

func (t *treeTX) Close() error {
	if !t.closed {
		err := t.Rollback()
		if err != nil {
			glog.Warningf("Rollback error on Close(): %v", err)
		}
		return err
	}
	return nil
}

func (t *treeTX) IsOpen() bool {
	return !t.closed
}

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
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/btree"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring/metric"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/trees"
)

var (
	defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

	queuedCounter   = metric.NewCounter("mem_queued_leaves")
	dequeuedCounter = metric.NewCounter("mem_dequeued_leaves")
)

func unseqKey(treeID int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/unseq", treeID)}
}

func seqLeafKey(treeID, seq int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/seq/%020d", treeID, seq)}
}

func hashToSeqKey(treeID int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/h2s")}
}

func sthKey(treeID, timestamp int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/sth/%020d", treeID, timestamp)}
}

type memoryLogStorage struct {
	*memoryTreeStorage
	admin storage.AdminStorage
}

// NewLogStorage creates an in-memory LogStorage instance.
func NewLogStorage() storage.LogStorage {
	ret := &memoryLogStorage{
		memoryTreeStorage: newTreeStorage(),
	}
	ret.admin = NewAdminStorage(ret)
	return ret
}

func (m *memoryLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}

type readOnlyLogTX struct {
	ms *memoryTreeStorage
}

func (m *memoryLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	return &readOnlyLogTX{m.memoryTreeStorage}, nil
}

func (t *readOnlyLogTX) Commit() error {
	return nil
}

func (t *readOnlyLogTX) Rollback() error {
	return nil
}

func (t *readOnlyLogTX) Close() error {
	return nil
}

func (t *readOnlyLogTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	t.ms.mu.RLock()
	defer t.ms.mu.RUnlock()

	ret := make([]int64, 0, len(t.ms.trees))
	for k := range t.ms.trees {
		ret = append(ret, k)
	}
	return ret, nil
}

func (t *readOnlyLogTX) GetActiveLogIDsWithPendingWork(ctx context.Context) ([]int64, error) {
	// just return all trees for now
	return t.GetActiveLogIDs(ctx)
}

func (m *memoryLogStorage) beginInternal(ctx context.Context, treeID int64, readonly bool) (storage.LogTreeTX, error) {
	tree, err := trees.GetTree(
		ctx,
		m.admin,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: readonly})
	if err != nil {
		return nil, err
	}
	hasher, err := trees.Hasher(tree)
	if err != nil {
		return nil, err
	}

	stCache := cache.NewSubtreeCache(defaultLogStrata, cache.PopulateLogSubtreeNodes(hasher), cache.PrepareLogSubtreeWrite())
	ttx, err := m.memoryTreeStorage.beginTreeTX(ctx, readonly, treeID, hasher.Size(), stCache)
	if err != nil {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX: ttx,
		ls:     m,
	}

	ltx.root, err = ltx.fetchLatestRoot(ctx)
	if err != nil {
		ttx.Rollback()
		return nil, err
	}
	ltx.treeTX.writeRevision = ltx.root.TreeRevision + 1

	return ltx, nil
}

func (m *memoryLogStorage) BeginForTree(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	return m.beginInternal(ctx, treeID, false /* readonly */)
}

func (m *memoryLogStorage) SnapshotForTree(ctx context.Context, treeID int64) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, treeID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTreeTX), err
}

type logTreeTX struct {
	treeTX
	ls   *memoryLogStorage
	root trillian.SignedLogRoot
}

func (t *logTreeTX) ReadRevision() int64 {
	return t.root.TreeRevision
}

func (t *logTreeTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
}

func (t *logTreeTX) DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error) {
	leaves := make([]*trillian.LogLeaf, 0, limit)

	q := t.tx.Get(unseqKey(t.treeID)).(*kv).v.(*list.List)
	e := q.Front()
	for i := 0; i < limit && e != nil; i++ {
		// TODO(al): consider cutoffTime
		leaves = append(leaves, e.Value.(*trillian.LogLeaf))
		e = e.Next()
	}

	return leaves, nil
}

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
	}
	// No deduping in this storage!
	k := unseqKey(t.treeID)
	q := t.tx.Get(k).(*kv).v.(*list.List)
	for _, l := range leaves {
		q.PushBack(l)
	}
	return []*trillian.LogLeaf{}, nil
}

func (t *logTreeTX) GetSequencedLeafCount(ctx context.Context) (int64, error) {
	var sequencedLeafCount int64

	t.tx.DescendRange(seqLeafKey(t.treeID, math.MaxInt64), seqLeafKey(t.treeID, 0), func(i btree.Item) bool {
		sequencedLeafCount = i.(*kv).v.(*trillian.LogLeaf).LeafIndex + 1
		return false
	})
	return sequencedLeafCount, nil
}

func (t *logTreeTX) GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error) {

	ret := make([]*trillian.LogLeaf, 0, len(leaves))
	for _, seq := range leaves {
		leaf := t.tx.Get(seqLeafKey(t.treeID, seq))
		if leaf != nil {
			ret = append(ret, leaf.(*kv).v.(*trillian.LogLeaf))
		}
	}
	return ret, nil
}

func (t *logTreeTX) GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error) {
	m := t.tx.Get(hashToSeqKey(t.treeID)).(*kv).v.(map[string][]int64)

	ret := make([]*trillian.LogLeaf, 0, len(leafHashes))
	for hash := range leafHashes {
		seq, ok := m[string(hash)]
		if !ok {
			continue
		}
		for _, s := range seq {
			l := t.tx.Get(seqLeafKey(t.treeID, s))
			if l == nil {
				continue
			}
			ret = append(ret, l.(*kv).v.(*trillian.LogLeaf))
		}
	}
	return ret, nil
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	return t.root, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	r := t.tx.Get(sthKey(t.treeID, t.tree.currentSTH))
	if r == nil {
		return trillian.SignedLogRoot{}, nil
	}
	return r.(*kv).v.(trillian.SignedLogRoot), nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, root trillian.SignedLogRoot) error {
	k := sthKey(t.treeID, root.TimestampNanos)
	k.(*kv).v = root
	t.tx.ReplaceOrInsert(k)

	// TODO(alcutter): this breaks the transactional model
	if root.TimestampNanos > t.tree.currentSTH {
		t.tree.currentSTH = root.TimestampNanos
	}
	return nil
}

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	countByMerkleHash := make(map[string]int)
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}
		mh := string(leaf.MerkleLeafHash)
		countByMerkleHash[mh]++
		// insert sequenced leaf:
		k := seqLeafKey(t.treeID, leaf.LeafIndex)
		k.(*kv).v = leaf
		t.tx.ReplaceOrInsert(k)
		// update merkle-to-seq mapping:
		m := t.tx.Get(hashToSeqKey(t.treeID))
		l := m.(*kv).v.(map[string][]int64)[string(leaf.MerkleLeafHash)]
		l = append(l, leaf.LeafIndex)
		m.(*kv).v.(map[string][]int64)[string(leaf.MerkleLeafHash)] = l
	}

	q := t.tx.Get(unseqKey(t.treeID)).(*kv).v.(*list.List)
	toRemove := make([]*list.Element, 0, q.Len())
	for e := q.Front(); e != nil && len(countByMerkleHash) > 0; e = e.Next() {
		h := e.Value.(*trillian.LogLeaf).MerkleLeafHash
		mh := string(h)
		if countByMerkleHash[mh] > 0 {
			countByMerkleHash[mh]--
			toRemove = append(toRemove, e)
			if countByMerkleHash[mh] == 0 {
				delete(countByMerkleHash, mh)
			}
		}
	}
	for _, e := range toRemove {
		q.Remove(e)
	}

	if unknown := len(countByMerkleHash); unknown != 0 {
		return fmt.Errorf("attempted to update %d unknown leaves: %x", unknown, countByMerkleHash)
	}

	return nil
}

func (t *logTreeTX) getActiveLogIDs(ctx context.Context) ([]int64, error) {
	var ret []int64
	for k := range t.ts.trees {
		ret = append(ret, k)
	}
	return ret, nil
}

// GetActiveLogIDs returns a list of the IDs of all configured logs
func (t *logTreeTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	return t.getActiveLogIDs(ctx)
}

// GetActiveLogIDsWithPendingWork returns a list of the IDs of all configured logs
// that have queued unsequenced leaves that need to be integrated
func (t *logTreeTX) GetActiveLogIDsWithPendingWork(ctx context.Context) ([]int64, error) {
	// TODO(alcutter): only return trees with work to do
	return t.getActiveLogIDs(ctx)
}

// byLeafIdentityHash allows sorting of leaves by their identity hash, so DB
// operations always happen in a consistent order.
type byLeafIdentityHash []*trillian.LogLeaf

func (l byLeafIdentityHash) Len() int {
	return len(l)
}
func (l byLeafIdentityHash) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
func (l byLeafIdentityHash) Less(i, j int) bool {
	return bytes.Compare(l[i].LeafIdentityHash, l[j].LeafIdentityHash) == -1
}

// leafAndPosition records original position before sort.
type leafAndPosition struct {
	leaf *trillian.LogLeaf
	idx  int
}

// byLeafIdentityHashWithPosition allows sorting (as above), but where we need
// to remember the original position
type byLeafIdentityHashWithPosition []leafAndPosition

func (l byLeafIdentityHashWithPosition) Len() int {
	return len(l)
}
func (l byLeafIdentityHashWithPosition) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
func (l byLeafIdentityHashWithPosition) Less(i, j int) bool {
	return bytes.Compare(l[i].leaf.LeafIdentityHash, l[j].leaf.LeafIdentityHash) == -1
}

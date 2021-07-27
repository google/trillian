// Copyright 2016 Google LLC. All Rights Reserved.
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

package cache

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"sync"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"google.golang.org/protobuf/proto"
)

// TODO(al): move this up the stack
var populateConcurrency = flag.Int("populate_subtree_concurrency", 256, "Max number of concurrent workers concurrently populating subtrees")

// TODO(pavelkalinnikov): Rename subtrees to tiles.

// GetSubtreesFunc describes a function which can return a number of Subtrees from storage.
type GetSubtreesFunc func(ids [][]byte) ([]*storagepb.SubtreeProto, error)

// SetSubtreesFunc describes a function which can store a collection of Subtrees into storage.
type SetSubtreesFunc func(ctx context.Context, s []*storagepb.SubtreeProto) error

// SubtreeCache provides a caching access to Subtree storage. Currently there are assumptions
// in the code that all subtrees are multiple of 8 in depth and that log subtrees are always
// of depth 8. It is not possible to just change the constants above and have things still
// work. This is because of issues like byte packing of node IDs.
//
// The cache is optimized for the following use-cases (see sync.Map type for more details):
//  1. Parallel readers/writers working on non-intersecting subsets of subtrees/nodes.
//  2. Subtrees/nodes are rarely written, and mostly read.
type SubtreeCache struct {
	hasher hashers.LogHasher

	// subtrees contains the Subtree data read from storage, and is updated by
	// calls to SetNodes.
	subtrees sync.Map
	// dirtyPrefixes keeps track of all Subtrees which need to be written back
	// to storage.
	dirtyPrefixes sync.Map

	// populateConcurrency sets the amount of concurrency when repopulating subtrees.
	populateConcurrency int
}

// NewLogSubtreeCache creates and returns a SubtreeCache appropriate for use with a log
// tree. The caller must supply a suitable LogHasher.
func NewLogSubtreeCache(hasher hashers.LogHasher) *SubtreeCache {
	if *populateConcurrency <= 0 {
		panic(fmt.Errorf("populate_subtree_concurrency must be set to >= 1"))
	}
	return &SubtreeCache{
		hasher:              hasher,
		populateConcurrency: *populateConcurrency,
	}
}

// preload calculates the set of subtrees required to know the hashes of the
// passed in node IDs, uses getSubtrees to retrieve them, and finally populates
// the cache structures with the data. Returns the list of tile IDs not found.
func (s *SubtreeCache) preload(ids []compact.NodeID, getSubtrees GetSubtreesFunc) ([]string, error) {
	// Figure out the set of subtrees we need.
	want := make(map[string]bool)
	for _, id := range ids {
		subID := string(getTileID(id))
		if _, ok := want[subID]; ok {
			// No need to check s.subtrees map twice.
			continue
		}
		if _, ok := s.subtrees.Load(subID); !ok {
			want[subID] = true
		}
	}
	// Note: At this point multiple parallel preload invocations can happen to
	// getSubtrees with overlapping sets of IDs. It's okay because we collapse
	// results further below.

	// Don't make a read request for zero subtrees.
	if len(want) == 0 {
		return nil, nil
	}

	list := make([][]byte, 0, len(want))
	for id := range want {
		list = append(list, []byte(id))
	}
	subtrees, err := getSubtrees(list)
	if err != nil {
		return nil, err
	}
	if got, max := len(subtrees), len(want); got > max {
		return nil, fmt.Errorf("too many subtrees: %d, want <= %d", got, max)
	}

	ch := make(chan *storagepb.SubtreeProto, len(subtrees))
	workTokens := make(chan bool, s.populateConcurrency)
	for i := 0; i < s.populateConcurrency; i++ {
		workTokens <- true
	}
	wg := &sync.WaitGroup{}

	for _, t := range subtrees {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			// wait for a token before starting work
			<-workTokens
			// return it when done
			defer func() { workTokens <- true }()

			PopulateLogTile(t, s.hasher)
			ch <- t // Note: This never blocks because len(ch) == len(subtrees).
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
		close(workTokens)
	}()

	for t := range ch {
		if err := s.cacheSubtree(t); err != nil {
			return nil, err
		}
		delete(want, string(t.Prefix))
	}
	notFound := make([]string, len(want))
	for id := range want {
		notFound = append(notFound, id)
	}
	return notFound, nil
}

func (s *SubtreeCache) cacheSubtree(t *storagepb.SubtreeProto) error {
	raw, loaded := s.subtrees.LoadOrStore(string(t.Prefix), t)
	if loaded { // There is already something with this key.
		if subtree, ok := raw.(*storagepb.SubtreeProto); !ok {
			return fmt.Errorf("at %x: not a subtree: %T", t.Prefix, raw)
		} else if !proto.Equal(t, subtree) {
			return fmt.Errorf("at %x: subtree mismatch", t.Prefix)
		}
	}
	return nil
}

// GetNodes returns the requested nodes, calling the getSubtrees function if
// they are not already cached.
func (s *SubtreeCache) GetNodes(ids []compact.NodeID, getSubtrees GetSubtreesFunc) ([]tree.Node, error) {
	if notFound, err := s.preload(ids, getSubtrees); err != nil {
		return nil, err
	} else if r := len(notFound); r != 0 {
		return nil, fmt.Errorf("preload did not get all tiles: %d not found", r)
	}

	ret := make([]tree.Node, 0, len(ids))
	for _, id := range ids {
		if h, err := s.getNodeHash(id); err != nil {
			return nil, fmt.Errorf("getNodeHash(%+v): %v", id, err)
		} else if h != nil {
			ret = append(ret, tree.Node{ID: id, Hash: h})
		}
	}
	return ret, nil
}

func (s *SubtreeCache) getCachedSubtree(id []byte) *storagepb.SubtreeProto {
	raw, found := s.subtrees.Load(string(id))
	if !found || raw == nil {
		return nil
	}
	// Note: If type assertion fails, nil is returned. Should not happen though.
	res, _ := raw.(*storagepb.SubtreeProto)
	return res
}

func (s *SubtreeCache) prefixIsDirty(prefixKey string) bool {
	_, found := s.dirtyPrefixes.Load(prefixKey)
	return found
}

// getNodeHash returns a single node hash from the cache.
func (s *SubtreeCache) getNodeHash(id compact.NodeID) ([]byte, error) {
	subID, sx := splitID(id)
	c := s.getCachedSubtree(subID)
	if c == nil {
		return nil, fmt.Errorf("tile %x not found", subID)
	}

	// Look up the hash in the appropriate map.
	// The leaf hashes are stored in a separate map to the internal nodes so that
	// we can easily dump (and later reconstruct) the internal nodes. As log subtrees
	// have a fixed depth if the suffix has the same number of significant bits as the
	// subtree depth then this is a leaf. For example if the subtree is depth 8 its leaves
	// have 8 significant suffix bits.
	if int32(sx.Bits()) == c.Depth {
		return c.Leaves[sx.String()], nil
	}
	return c.InternalNodes[sx.String()], nil
}

// SetNodes sets hashes for the given nodes in the cache.
func (s *SubtreeCache) SetNodes(nodes []tree.Node, getSubtrees GetSubtreesFunc) error {
	ids := make([]compact.NodeID, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	notFound, err := s.preload(ids, getSubtrees)
	if err != nil {
		return err
	}
	for _, id := range notFound {
		c := newEmptyTile([]byte(id))
		s.subtrees.Store(string(id), c)
	}

	for _, n := range nodes {
		subID, sx := splitID(n.ID)
		c := s.getCachedSubtree(subID)
		if c == nil {
			return fmt.Errorf("tile %x not found", subID)
		}

		// Store the hash to the containing tile, and mark it as dirty if the hash
		// differs from the previously stored one.
		sfxKey := sx.String()
		if int32(sx.Bits()) == c.Depth { // This is a leaf node.
			if !bytes.Equal(c.Leaves[sfxKey], n.Hash) {
				c.Leaves[sfxKey] = n.Hash
				s.dirtyPrefixes.Store(string(subID), nil)
			}
		} else { // This is an internal node.
			if !bytes.Equal(c.InternalNodes[sfxKey], n.Hash) {
				c.InternalNodes[sfxKey] = n.Hash
				s.dirtyPrefixes.Store(string(subID), nil)
			}
		}
	}

	return nil
}

// Flush causes the cache to write all dirty Subtrees back to storage.
func (s *SubtreeCache) Flush(ctx context.Context, setSubtrees SetSubtreesFunc) error {
	treesToWrite := make([]*storagepb.SubtreeProto, 0)
	var rangeErr error
	s.subtrees.Range(func(rawK, rawV interface{}) bool {
		k, ok := rawK.(string)
		if !ok {
			rangeErr = fmt.Errorf("unknown key type: %t", rawV)
			return false
		}
		v, ok := rawV.(*storagepb.SubtreeProto)
		if !ok {
			rangeErr = fmt.Errorf("unknown value type: %t", rawV)
			return false
		}
		if s.prefixIsDirty(k) {
			bk := []byte(k)
			if !bytes.Equal(bk, v.Prefix) {
				rangeErr = fmt.Errorf("inconsistent cache: prefix key is %v, but cached object claims %v", bk, v.Prefix)
				return false
			}

			if len(v.Leaves) > 0 {
				if err := prepareLogTile(v); err != nil {
					rangeErr = err
					return false
				}
				treesToWrite = append(treesToWrite, v)
			}
		}
		return true
	})
	if rangeErr != nil {
		return rangeErr
	}
	if len(treesToWrite) == 0 {
		return nil
	}
	err := setSubtrees(ctx, treesToWrite)
	return err
}

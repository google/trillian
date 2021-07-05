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

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"google.golang.org/protobuf/proto"
)

// TODO(al): move this up the stack
var populateConcurrency = flag.Int("populate_subtree_concurrency", 256, "Max number of concurrent workers concurrently populating subtrees")

// TODO(pavelkalinnikov): Rename subtrees to tiles.

// GetSubtreeFunc describes a function which can return a Subtree from storage.
type GetSubtreeFunc func(id []byte) (*storagepb.SubtreeProto, error)

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
	// calls to SetNodeHash.
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
// the cache structures with the data.
func (s *SubtreeCache) preload(ids []compact.NodeID, getSubtrees GetSubtreesFunc) error {
	// Figure out the set of subtrees we need.
	want := make(map[string]bool)
	for _, id := range ids {
		subID := string(tree.GetTileID(id))
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
		return nil
	}

	list := make([][]byte, 0, len(want))
	for id := range want {
		list = append(list, []byte(id))
	}
	subtrees, err := getSubtrees(list)
	if err != nil {
		return err
	}
	if got, max := len(subtrees), len(want); got > max {
		return fmt.Errorf("too many subtrees: %d, want <= %d", got, max)
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
			return err
		}
		delete(want, string(t.Prefix))
	}
	if r := len(want); r != 0 {
		return fmt.Errorf("preload did not get all tiles: %d not found", r)
	}
	return nil
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
	if err := s.preload(ids, getSubtrees); err != nil {
		return nil, err
	}

	ret := make([]tree.Node, 0, len(ids))
	for _, id := range ids {
		h, err := s.getNodeHash(
			id,
			func(id []byte) (*storagepb.SubtreeProto, error) {
				// This should never happen - we should've already read all the data we
				// need above, in Preload()
				glog.Warningf("Unexpectedly reading from within getNodeHash(): %x", id)
				ret, err := getSubtrees([][]byte{id})
				if err != nil || len(ret) == 0 {
					return nil, err
				}
				if n := len(ret); n > 1 {
					return nil, fmt.Errorf("got %d trees, want: 1", n)
				}
				return ret[0], nil
			})
		if err != nil {
			return nil, err
		}

		if h != nil {
			ret = append(ret, tree.Node{
				ID:   id,
				Hash: h,
			})
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
func (s *SubtreeCache) getNodeHash(id compact.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	subID, sx := tree.Split(id)
	c := s.getCachedSubtree(subID)
	if c == nil {
		glog.V(2).Infof("Cache miss for %x so we'll try to fetch from storage", subID)
		// Cache miss, so we'll try to fetch from storage.
		var err error
		if c, err = getSubtree(subID); err != nil {
			return nil, err
		}
		if c == nil {
			c = newEmptySubtree(subID)
		} else {
			if err := PopulateLogTile(c, s.hasher); err != nil {
				return nil, err
			}
		}
		if c.Prefix == nil {
			return nil, fmt.Errorf("getNodeHash nil prefix on %v for id %+v with px %x", c, id, subID)
		}

		s.subtrees.Store(string(subID), c)
	}

	// finally look for the particular node within the subtree so we can return
	// the hash & revision.
	var nh []byte

	// Look up the hash in the appropriate map.
	// The leaf hashes are stored in a separate map to the internal nodes so that
	// we can easily dump (and later reconstruct) the internal nodes. As log subtrees
	// have a fixed depth if the suffix has the same number of significant bits as the
	// subtree depth then this is a leaf. For example if the subtree is depth 8 its leaves
	// have 8 significant suffix bits.
	sfxKey := sx.String()
	if int32(sx.Bits()) == c.Depth {
		nh = c.Leaves[sfxKey]
	} else {
		nh = c.InternalNodes[sfxKey]
	}
	return nh, nil
}

// SetNodeHash sets a node hash in the cache.
func (s *SubtreeCache) SetNodeHash(id compact.NodeID, h []byte, getSubtree GetSubtreeFunc) error {
	subID, sx := tree.Split(id)
	c := s.getCachedSubtree(subID)
	if c == nil {
		// TODO(al): This is ok, IFF *all* leaves in the subtree are being set,
		// verify that this is the case when it happens.
		// For now, just read from storage if we don't already have it.
		glog.V(1).Infof("attempting to write to unread subtree for %+v, reading now", id)
		if _, err := s.getNodeHash(id, getSubtree); err != nil {
			return err
		}
		// There must be a subtree present in the cache now, even if storage didn't have anything for us.
		c = s.getCachedSubtree(subID)
		if c == nil {
			return fmt.Errorf("internal error, subtree cache for %+v is nil after a read attempt", id)
		}
	}
	if c.Prefix == nil {
		return fmt.Errorf("nil prefix for %+v (key %v)", id, subID)
	}
	// Determine whether we're being asked to store a leaf node, or an internal
	// node, and store it accordingly.
	sfxKey := sx.String()
	if int32(sx.Bits()) == c.Depth {
		// If the value being set is identical to the one we read from storage, then
		// leave the cache state alone, and return.  This will prevent a write (and
		// subtree revision bump) for identical data.
		if bytes.Equal(c.Leaves[sfxKey], h) {
			return nil
		}
		c.Leaves[sfxKey] = h
	} else {
		// If the value being set is identical to the one we read from storage, then
		// leave the cache state alone, and return.  This will prevent a write (and
		// subtree revision bump) for identical data.
		if bytes.Equal(c.InternalNodes[sfxKey], h) {
			return nil
		}
		c.InternalNodes[sfxKey] = h
	}
	s.dirtyPrefixes.Store(string(subID), nil)
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
			// TODO(al): Do actually write this one once we're storing the updated
			// subtree root value here during tree update calculations.
			v.RootHash = nil

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

// newEmptySubtree creates an empty subtree for the passed-in ID.
func newEmptySubtree(id []byte) *storagepb.SubtreeProto {
	if glog.V(2) {
		glog.Infof("Creating new empty subtree for %x", id)
	}
	// Storage didn't have one for us, so we'll store an empty proto here in case
	// we try to update it later on (we won't flush it back to storage unless
	// it's been written to).
	return &storagepb.SubtreeProto{
		Prefix:        id,
		Depth:         8,
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
	}
}

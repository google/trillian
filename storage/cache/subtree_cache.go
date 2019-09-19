// Copyright 2016 Google Inc. All Rights Reserved.
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
	"encoding/base64"
	"flag"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
)

// TODO(al): move this up the stack
var populateConcurrency = flag.Int("populate_subtree_concurrency", 256, "Max number of concurrent workers concurrently populating subtrees")

// TODO(pavelkalinnikov): Rename subtrees to tiles.

// GetSubtreeFunc describes a function which can return a Subtree from storage.
type GetSubtreeFunc func(id tree.NodeID) (*storagepb.SubtreeProto, error)

// GetSubtreesFunc describes a function which can return a number of Subtrees from storage.
type GetSubtreesFunc func(ids []tree.NodeID) ([]*storagepb.SubtreeProto, error)

// SetSubtreesFunc describes a function which can store a collection of Subtrees into storage.
type SetSubtreesFunc func(ctx context.Context, s []*storagepb.SubtreeProto) error

// maxSupportedTreeDepth is the maximum depth a tree can reach. Note that log
// trees are further limited to a depth of 63 by the use of signed 64 bit leaf
// indices. Map trees do not have this restriction.
const maxSupportedTreeDepth = 256

// SubtreeCache provides a caching access to Subtree storage. Currently there are assumptions
// in the code that all subtrees are multiple of 8 in depth and that log subtrees are always
// of depth 8. It is not possible to just change the constants above and have things still
// work. This is because of issues like byte packing of node IDs.
//
// The cache is optimized for the following use-cases (see sync.Map type for more details):
//  1. Parallel readers/writers working on non-intersecting subsets of subtrees/nodes.
//  2. Subtrees/nodes are rarely written, and mostly read.
type SubtreeCache struct {
	layout *tree.Layout

	// subtrees contains the Subtree data read from storage, and is updated by
	// calls to SetNodeHash.
	subtrees sync.Map
	// dirtyPrefixes keeps track of all Subtrees which need to be written back
	// to storage.
	dirtyPrefixes sync.Map

	// populate is used to rebuild internal nodes when subtrees are loaded from storage.
	populate tree.PopulateSubtreeFunc
	// populateConcurrency sets the amount of concurrency when repopulating subtrees.
	populateConcurrency int
	// prepare is used for preparation work when subtrees are about to be written to storage.
	prepare tree.PrepareSubtreeWriteFunc
}

// NewSubtreeCache returns a newly intialised cache ready for use.
// populateSubtree is a function which knows how to populate a subtree's
// internal nodes given its leaves, and will be called for each subtree loaded
// from storage.
func NewSubtreeCache(strataDepths []int, populateSubtree tree.PopulateSubtreeFunc, prepareSubtreeWrite tree.PrepareSubtreeWriteFunc) *SubtreeCache {
	// TODO(al): pass this in
	maxTreeDepth := maxSupportedTreeDepth
	glog.V(1).Infof("Creating new subtree cache maxDepth=%d strataDepths=%v", maxTreeDepth, strataDepths)
	layout := tree.NewLayout(strataDepths)

	// TODO(al): This needs to be passed in, particularly for Map use cases where
	// we need to know it matches the number of bits in the chosen hash function.
	if got, want := layout.Height, maxTreeDepth; got != want {
		panic(fmt.Errorf("strata indicate tree of depth %d, but expected %d", got, want))
	}

	if *populateConcurrency <= 0 {
		panic(fmt.Errorf("populate_subtree_concurrency must be set to >= 1"))
	}

	return &SubtreeCache{
		layout:              layout,
		populate:            populateSubtree,
		populateConcurrency: *populateConcurrency,
		prepare:             prepareSubtreeWrite,
	}
}

// preload calculates the set of subtrees required to know the hashes of the
// passed in node IDs, uses getSubtrees to retrieve them, and finally populates
// the cache structures with the data.
func (s *SubtreeCache) preload(ids []tree.NodeID, getSubtrees GetSubtreesFunc) error {
	// Figure out the set of subtrees we need.
	want := make(map[string]tree.TileID)
	for _, id := range ids {
		subID := s.layout.GetTileID(id)
		subKey := subID.AsKey()
		if _, ok := want[subKey]; ok {
			// No need to check s.subtrees map twice.
			continue
		}
		if _, ok := s.subtrees.Load(subKey); !ok {
			want[subKey] = subID
		}
	}
	// Note: At this point multiple parallel preload invocations can happen to
	// getSubtrees with overlapping sets of IDs. It's okay because we collapse
	// results further below.

	// Don't make a read request for zero subtrees.
	if len(want) == 0 {
		return nil
	}

	// TODO(pavelkalinnikov): Change the getters to accept []tree.TileID.
	list := make([]tree.NodeID, 0, len(want))
	for _, v := range want {
		list = append(list, v.Root)
	}
	subtrees, err := getSubtrees(list)
	if err != nil {
		return err
	}

	ch := make(chan *storagepb.SubtreeProto, len(want))
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

			s.populate(t)
			ch <- t
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

	// We might not have got all the subtrees we requested, if they don't already exist.
	// Create empty subtrees for anything left over. Note that multiple parallel readers
	// may be be running this code and touch the same keys, although this doesn't happen
	// normally.
	for _, id := range want {
		if err := s.cacheSubtree(s.newEmptySubtree(id)); err != nil {
			return err
		}
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
func (s *SubtreeCache) GetNodes(ids []tree.NodeID, getSubtrees GetSubtreesFunc) ([]tree.Node, error) {
	glog.V(2).Infof("cache: GetNodes(count=%d)", len(ids))
	if glog.V(3) {
		for _, n := range ids {
			glog.Infof("  cache: GetNodes(path=%x, prefixLen=%d)", n.Path, n.PrefixLenBits)
		}
	}
	if err := s.preload(ids, getSubtrees); err != nil {
		return nil, err
	}

	ret := make([]tree.Node, 0, len(ids))
	for _, id := range ids {
		h, err := s.getNodeHash(
			id,
			func(n tree.NodeID) (*storagepb.SubtreeProto, error) {
				// This should never happen - we should've already read all the data we
				// need above, in Preload()
				glog.Warningf("Unexpectedly reading from within getNodeHash(): %s", n.String())
				ret, err := getSubtrees([]tree.NodeID{n})
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
				NodeID: id,
				Hash:   h,
			})
		}
	}
	glog.V(2).Infof("cache: GetNodes(count=%d) => %d results", len(ids), len(ret))
	if glog.V(3) {
		for _, r := range ret {
			glog.Infof("  cache: Node{rev=%d, path=%x, prefixLen=%d, hash=%x}", r.NodeRevision, r.NodeID.Path, r.NodeID.PrefixLenBits, r.Hash)
		}
	}
	return ret, nil
}

func (s *SubtreeCache) getCachedSubtree(prefixKey string) *storagepb.SubtreeProto {
	raw, found := s.subtrees.Load(prefixKey)
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
func (s *SubtreeCache) getNodeHash(id tree.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) {", id.Path, id.PrefixLenBits)
	}

	subID, sx := s.layout.Split(id)
	subKey := subID.AsKey()
	c := s.getCachedSubtree(subKey)
	if c == nil {
		glog.V(2).Infof("Cache miss for %x so we'll try to fetch from storage", subKey)
		// Cache miss, so we'll try to fetch from storage.
		var err error
		if c, err = getSubtree(subID.Root); err != nil {
			return nil, err
		}
		if c == nil {
			c = s.newEmptySubtree(subID)
		} else {
			if err := s.populate(c); err != nil {
				return nil, err
			}
		}
		if c.Prefix == nil {
			panic(fmt.Errorf("getNodeHash nil prefix on %v for id %v with px %x", c, id.String(), subKey))
		}

		s.subtrees.Store(subKey, c)
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
	if glog.V(4) {
		b, err := base64.StdEncoding.DecodeString(sfxKey)
		if err != nil {
			glog.Errorf("base64.DecodeString(%v): %v", sfxKey, err)
		}
		glog.Infof("getNodeHash(%x | %x): %x", subKey, b, nh)
	}

	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) => %x}", id.Path, id.PrefixLenBits, nh)
	}
	return nh, nil
}

// SetNodeHash sets a node hash in the cache.
func (s *SubtreeCache) SetNodeHash(id tree.NodeID, h []byte, getSubtree GetSubtreeFunc) error {
	if glog.V(3) {
		glog.Infof("cache: SetNodeHash(%x, %d)=%x", id.Path, id.PrefixLenBits, h)
	}

	subID, sx := s.layout.Split(id)
	subKey := subID.AsKey()
	c := s.getCachedSubtree(subKey)
	if c == nil {
		// TODO(al): This is ok, IFF *all* leaves in the subtree are being set,
		// verify that this is the case when it happens.
		// For now, just read from storage if we don't already have it.
		glog.V(1).Infof("attempting to write to unread subtree for %v, reading now", id.String())
		if _, err := s.getNodeHash(id, getSubtree); err != nil {
			return err
		}
		// There must be a subtree present in the cache now, even if storage didn't have anything for us.
		c = s.getCachedSubtree(subKey)
		if c == nil {
			return fmt.Errorf("internal error, subtree cache for %v is nil after a read attempt", id.String())
		}
	}
	if c.Prefix == nil {
		return fmt.Errorf("nil prefix for %v (key %v)", id.String(), subKey)
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
	s.dirtyPrefixes.Store(subKey, nil)
	if glog.V(3) {
		b, err := base64.StdEncoding.DecodeString(sfxKey)
		if err != nil {
			glog.Errorf("base64.DecodeString(%v): %v", sfxKey, err)
		}
		glog.Infof("SetNodeHash(pfx: %x, sfx: %x): %x", subKey, b, h)
	}
	return nil
}

// Flush causes the cache to write all dirty Subtrees back to storage.
func (s *SubtreeCache) Flush(ctx context.Context, setSubtrees SetSubtreesFunc) error {
	glog.V(1).Info("cache: Flush")

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
				// prepare internal nodes ready for the write (tree type specific)
				if err := s.prepare(v); err != nil {
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
	glog.V(1).Infof("cache: Flush done %v", err)
	return err
}

// newEmptySubtree creates an empty subtree for the passed-in ID.
func (s *SubtreeCache) newEmptySubtree(id tree.TileID) *storagepb.SubtreeProto {
	height := s.layout.GetTileHeight(id)
	if glog.V(2) {
		glog.Infof("Creating new empty subtree for %x, with height %d", id.AsBytes(), height)
	}
	// Storage didn't have one for us, so we'll store an empty proto here in case
	// we try to update it later on (we won't flush it back to storage unless
	// it's been written to).
	return &storagepb.SubtreeProto{
		Prefix:        id.AsBytes(),
		Depth:         int32(height),
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
	}
}

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
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
)

// TODO(al): move this up the stack
var populateConcurrency = flag.Int("populate_subtree_concurrency", 256, "Max number of concurrent workers concurrently populating subtrees")

// GetSubtreeFunc describes a function which can return a Subtree from storage.
type GetSubtreeFunc func(id storage.NodeID) (*storagepb.SubtreeProto, error)

// GetSubtreesFunc describes a function which can return a number of Subtrees from storage.
type GetSubtreesFunc func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error)

// SetSubtreesFunc describes a function which can store a collection of Subtrees into storage.
type SetSubtreesFunc func(ctx context.Context, s []*storagepb.SubtreeProto) error

// stratumInfo represents a single stratum across the tree.
// It it used inside the SubtreeCache to determine which Subtree prefix should
// be used for a given NodeID.
// Currently, the strata must have depths which are multiples of 8.
type stratumInfo struct {
	// prefixBytes is the number of prefix bytes above this stratum.
	prefixBytes int
	// depth is the number of levels in this stratum.
	depth int
}

const (
	// maxSupportedTreeDepth is the maximum depth a tree can reach. Note that log trees are
	// further limited to a depth of 63 by the use of signed 64 bit leaf indices. Map trees
	// do not have this restriction.
	maxSupportedTreeDepth = 256
	// depthQuantum defines the smallest supported subtree depth and all subtrees must be
	// a multiple of this value in depth.
	depthQuantum = 8
	// logStrataDepth is the strata that must be used for all log subtrees.
	logStrataDepth = 8
	// maxLogDepth is the number of bits in a log path.
	maxLogDepth = 64
)

// SubtreeCache provides a caching access to Subtree storage. Currently there are assumptions
// in the code that all subtrees are multiple of 8 in depth and that log subtrees are always
// of depth 8. It is not possible to just change the constants above and have things still
// work. This is because of issues like byte packing of node IDs.
//
// The cache is optimized for the following use-cases (see sync.Map type for more details):
//  1. Parallel readers/writers working on non-intersecting subsets of subtrees/nodes.
//  2. Subtrees/nodes are rarely written, and mostly read.
type SubtreeCache struct {
	// prefixLengths contains the strata prefix sizes for each multiple-of-depthQuantum tree
	// size.
	stratumInfo []stratumInfo

	// subtrees contains the Subtree data read from storage, and is updated by
	// calls to SetNodeHash.
	subtrees sync.Map
	// dirtyPrefixes keeps track of all Subtrees which need to be written back
	// to storage.
	dirtyPrefixes sync.Map

	// populate is used to rebuild internal nodes when subtrees are loaded from storage.
	populate storage.PopulateSubtreeFunc
	// populateConcurrency sets the amount of concurrency when repopulating subtrees.
	populateConcurrency int
	// prepare is used for preparation work when subtrees are about to be written to storage.
	prepare storage.PrepareSubtreeWriteFunc
}

// NewSubtreeCache returns a newly intialised cache ready for use.
// populateSubtree is a function which knows how to populate a subtree's
// internal nodes given its leaves, and will be called for each subtree loaded
// from storage.
// TODO(al): consider supporting different sized subtrees - for now everything's subtrees of 8 levels.
func NewSubtreeCache(strataDepths []int, populateSubtree storage.PopulateSubtreeFunc, prepareSubtreeWrite storage.PrepareSubtreeWriteFunc) *SubtreeCache {
	// TODO(al): pass this in
	maxTreeDepth := maxSupportedTreeDepth
	glog.V(1).Infof("Creating new subtree cache maxDepth=%d strataDepths=%v", maxTreeDepth, strataDepths)
	// Precalculate strata information based on the passed in strata depths:
	sInfo := make([]stratumInfo, 0, maxTreeDepth/depthQuantum)
	t := 0
	for _, sDepth := range strataDepths {
		// Verify the stratum depth makes sense:
		if sDepth <= 0 {
			panic(fmt.Errorf("got invalid strata depth of %d: can't be <= 0", sDepth))
		}
		if sDepth%depthQuantum != 0 {
			panic(fmt.Errorf("got strata depth of %d, must be a multiple of %d", sDepth, depthQuantum))
		}

		pb := t / depthQuantum
		for i := 0; i < sDepth; i += depthQuantum {
			glog.V(2).Infof("  add stratumInfo={prefixBytes=%d, depth=%d}", pb, sDepth)
			sInfo = append(sInfo, stratumInfo{pb, sDepth})
			t += depthQuantum
		}
	}
	// TODO(al): This needs to be passed in, particularly for Map use cases where
	// we need to know it matches the number of bits in the chosen hash function.
	if got, want := t, maxTreeDepth; got != want {
		panic(fmt.Errorf("strata indicate tree of depth %d, but expected %d", got, want))
	}

	if *populateConcurrency <= 0 {
		panic(fmt.Errorf("populate_subtree_concurrency must be set to >= 1"))
	}

	return &SubtreeCache{
		stratumInfo:         sInfo,
		populate:            populateSubtree,
		populateConcurrency: *populateConcurrency,
		prepare:             prepareSubtreeWrite,
	}
}

func (s *SubtreeCache) stratumInfoForNodeID(id storage.NodeID) stratumInfo {
	return s.stratumInfoForPrefixLength(id.PrefixLenBits - 1)
}

func (s *SubtreeCache) stratumInfoForPrefixLength(l int) stratumInfo {
	return s.stratumInfo[l/depthQuantum]
}

// GetNodes returns the requested nodes. It calls the getSubtrees function to
// fetch any subtrees that are not already cached.
func (s *SubtreeCache) GetNodes(ids []storage.NodeID, getSubtrees GetSubtreesFunc) ([]storage.Node, error) {
	type subtree struct {
		id   storage.NodeID
		sp   *storagepb.SubtreeProto
		miss bool
	}
	subtrees := make(map[string]*subtree)
	// We need a node-to-subtree index in the end, to load individual hashes.
	index := make([]*subtree, len(ids))

	// Figure out the set of subtrees we need.
	// TODO(pavelkalinnikov): Pull this out of cache code.
	for i, id := range ids {
		sInfo := s.stratumInfoForNodeID(id)
		pxKey := id.PrefixAsKey(sInfo.prefixBytes)
		st, found := subtrees[pxKey]
		if !found {
			// TODO(al): Fix for non-uniform strata.
			id.PrefixLenBits = sInfo.prefixBytes * depthQuantum
			st = &subtree{id: id}
			subtrees[pxKey] = st
		}
		index[i] = st
	}

	// Filter out the subtrees that we already have in the cache.
	fetchIDs := make([]storage.NodeID, 0, len(subtrees))
	for pxKey, st := range subtrees {
		if sp := s.getCachedSubtree(pxKey); sp != nil {
			st.sp = sp // Save the subtree taken from the cache.
		} else { // Cache miss.
			fetchIDs = append(fetchIDs, st.id)
			st.miss = true
		}
	}

	// Fetch the subtrees that are not cached yet, if any.
	// Note: Multiple parallel readers might issue getSubtrees with overlapping
	// sets of IDs. It's okay because we collapse results further below.
	var fetched []*storagepb.SubtreeProto
	if len(fetchIDs) != 0 {
		var err error
		if fetched, err = getSubtrees(fetchIDs); err != nil {
			return nil, err
		}
	}

	// Populate all the fetched subtrees in parallel.
	// TODO(pavelkalinnikov): Consider using a semaphore.
	ch := make(chan *storagepb.SubtreeProto, len(fetched))
	workTokens := make(chan bool, s.populateConcurrency)
	for i := 0; i < s.populateConcurrency; i++ {
		workTokens <- true
	}
	var wg sync.WaitGroup
	for _, sp := range fetched {
		wg.Add(1)
		go func(sp *storagepb.SubtreeProto) {
			defer wg.Done()
			<-workTokens                          // Wait for a token.
			defer func() { workTokens <- true }() // Return it when done.
			s.populate(sp)
			ch <- sp
		}(sp)
	}
	// Clean up after all subtrees are populated.
	go func() {
		wg.Wait()
		close(ch)
		close(workTokens)
	}()
	// Save the populated subtrees.
	for sp := range ch {
		subtrees[string(sp.Prefix)].sp = sp
	}

	// Put all new subtrees into the cache.
	for pxKey, st := range subtrees {
		// We might not have fetched all the subtrees we requested, if some of them
		// don't exist. Create empty subtrees for anything left over.
		if st.sp == nil {
			st.sp = s.newEmptySubtree(st.id, []byte(pxKey))
		}
		if st.miss {
			// TODO(pavelkalinnikov): Do cacheSubtree calls asynchronously, or allow
			// the client to skip them.
			if err := s.cacheSubtree(st.sp); err != nil {
				return nil, err
			}
		}
	}

	// Get node hashes from the subtrees.
	ret := make([]storage.Node, len(ids))
	for i, id := range ids {
		sInfo := s.stratumInfoForNodeID(id)
		suf := id.Suffix(sInfo.prefixBytes, sInfo.depth)
		if h := getHashFromSubtree(index[i].sp, suf); h != nil {
			ret = append(ret, storage.Node{
				NodeID: id,
				Hash:   h,
			})
		}
	}

	glog.V(2).Infof("cache: GetNodes(count=%d) => %d results", len(ids), len(ret))
	return ret, nil
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
func (s *SubtreeCache) getNodeHash(id storage.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) {", id.Path, id.PrefixLenBits)
	}

	sInfo := s.stratumInfoForNodeID(id)
	prefixKey := id.PrefixAsKey(sInfo.prefixBytes)
	c := s.getCachedSubtree(prefixKey)
	if c == nil {
		glog.V(2).Infof("Cache miss for %x so we'll try to fetch from storage", prefixKey)
		px := id.Prefix(sInfo.prefixBytes)
		// Cache miss, so we'll try to fetch from storage.
		subID := id
		subID.PrefixLenBits = sInfo.prefixBytes * depthQuantum // this won't work if depthQuantum changes
		var err error
		c, err = getSubtree(subID)
		if err != nil {
			return nil, err
		}
		if c == nil {
			c = s.newEmptySubtree(subID, px)
		} else {
			if err := s.populate(c); err != nil {
				return nil, err
			}
		}
		if c.Prefix == nil {
			panic(fmt.Errorf("getNodeHash nil prefix on %v for id %v with px %#v", c, id.String(), px))
		}

		s.subtrees.Store(prefixKey, c)
	}

	// Finally look for the particular node within the subtree so we can return
	// the hash & revision.
	sx := id.Suffix(sInfo.prefixBytes, sInfo.depth)
	nh := getHashFromSubtree(c, sx)

	if glog.V(4) {
		b, err := base64.StdEncoding.DecodeString(sx.String())
		if err != nil {
			glog.Errorf("base64.DecodeString(%v): %v", sx, err)
		}
		glog.Infof("getNodeHash(%x | %x): %x", prefixKey, b, nh)
	}

	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) => %x}", id.Path, id.PrefixLenBits, nh)
	}
	return nh, nil
}

// SetNodeHash sets a node hash in the cache.
func (s *SubtreeCache) SetNodeHash(id storage.NodeID, h []byte, getSubtree GetSubtreeFunc) error {
	if glog.V(3) {
		glog.Infof("cache: SetNodeHash(%x, %d)=%x", id.Path, id.PrefixLenBits, h)
	}

	sInfo := s.stratumInfoForNodeID(id)
	prefixKey := id.PrefixAsKey(sInfo.prefixBytes)
	c := s.getCachedSubtree(prefixKey)
	if c == nil {
		// TODO(al): This is ok, IFF *all* leaves in the subtree are being set,
		// verify that this is the case when it happens.
		// For now, just read from storage if we don't already have it.
		glog.V(1).Infof("attempting to write to unread subtree for %v, reading now", id.String())
		if _, err := s.getNodeHash(id, getSubtree); err != nil {
			return err
		}
		// There must be a subtree present in the cache now, even if storage didn't have anything for us.
		c = s.getCachedSubtree(prefixKey)
		if c == nil {
			return fmt.Errorf("internal error, subtree cache for %v is nil after a read attempt", id.String())
		}
	}
	if c.Prefix == nil {
		return fmt.Errorf("nil prefix for %v (key %v)", id.String(), prefixKey)
	}
	// Determine whether we're being asked to store a leaf node, or an internal
	// node, and store it accordingly.
	sx := id.Suffix(sInfo.prefixBytes, sInfo.depth)
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
	s.dirtyPrefixes.Store(prefixKey, nil)
	if glog.V(3) {
		b, err := base64.StdEncoding.DecodeString(sfxKey)
		if err != nil {
			glog.Errorf("base64.DecodeString(%v): %v", sfxKey, err)
		}
		glog.Infof("SetNodeHash(pfx: %x, sfx: %x): %x", prefixKey, b, h)
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

func (s *SubtreeCache) newEmptySubtree(id storage.NodeID, px []byte) *storagepb.SubtreeProto {
	sInfo := s.stratumInfoForPrefixLength(id.PrefixLenBits)
	if glog.V(2) {
		glog.Infof("Creating new empty subtree for %x, with depth %d", px, sInfo.depth)
	}
	// storage didn't have one for us, so we'll store an empty proto here
	// incase we try to update it later on (we won't flush it back to
	// storage unless it's been written to.)
	return &storagepb.SubtreeProto{
		Prefix:        px,
		Depth:         int32(sInfo.depth),
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
	}
}

// getHashFromSubtree returns the hash of a node identified by the passed-in
// suffix within the provided subtree.
func getHashFromSubtree(sp *storagepb.SubtreeProto, suf *storage.Suffix) []byte {
	// Look up the hash in the appropriate map. The leaf hashes are stored in a
	// separate map to the internal nodes so that we can easily dump (and later
	// reconstruct) the internal nodes. If the suffix has the same number of
	// significant bits as the subtree depth then this is a leaf node.
	if int32(suf.Bits()) == sp.Depth {
		return sp.Leaves[suf.String()]
	}
	return sp.InternalNodes[suf.String()]
}

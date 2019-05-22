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
	"encoding/base64"
	"flag"
	"fmt"
	"sync"

	"github.com/golang/glog"
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
type SetSubtreesFunc func(s []*storagepb.SubtreeProto) error

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
type SubtreeCache struct {
	// prefixLengths contains the strata prefix sizes for each multiple-of-depthQuantum tree
	// size.
	stratumInfo []stratumInfo
	// subtrees contains the Subtree data read from storage, and is updated by
	// calls to SetNodeHash.
	subtrees map[string]*storagepb.SubtreeProto
	// dirtyPrefixes keeps track of all Subtrees which need to be written back
	// to storage.
	dirtyPrefixes map[string]bool
	// mutex guards access to the maps above.
	mutex *sync.RWMutex
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
func NewSubtreeCache(strataDepths []int, populateSubtree storage.PopulateSubtreeFunc, prepareSubtreeWrite storage.PrepareSubtreeWriteFunc) SubtreeCache {
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

	return SubtreeCache{
		stratumInfo:         sInfo,
		subtrees:            make(map[string]*storagepb.SubtreeProto),
		dirtyPrefixes:       make(map[string]bool),
		mutex:               new(sync.RWMutex),
		populate:            populateSubtree,
		populateConcurrency: *populateConcurrency,
		prepare:             prepareSubtreeWrite,
	}
}

func (s *SubtreeCache) stratumInfoForPrefixLength(numBits int) stratumInfo {
	return s.stratumInfo[numBits/depthQuantum]
}

// splitNodeID breaks a NodeID out into its prefix and suffix parts.
// unless ID is 0 bits long, Suffix must always contain at least one bit.
func (s *SubtreeCache) splitNodeID(id storage.NodeID) ([]byte, *storage.Suffix) {
	sInfo := s.stratumInfoForPrefixLength(id.PrefixLenBits - 1)
	return id.Split(sInfo.prefixBytes, sInfo.depth)
}

// preload calculates the set of subtrees required to know the hashes of the
// passed in node IDs, uses getSubtrees to retrieve them, and finally populates
// the cache structures with the data.
func (s *SubtreeCache) preload(ids []storage.NodeID, getSubtrees GetSubtreesFunc) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Figure out the set of subtrees we need:
	want := make(map[string]*storage.NodeID)
	for _, id := range ids {
		id := id
		// We can use a prefix of the ID path directly rather than splitting the ID.
		// We only need it to derive a key and we don't need the suffix.
		sInfo := s.stratumInfoForPrefixLength(id.PrefixLenBits - 1)
		pxLen := sInfo.prefixBytes
		pxKey := string(id.Path[:pxLen])
		// TODO(al): fix for non-uniform strata. Note that there is already a
		// baked-in assumption that the key derived from the prefix exactly
		// matches the subtree prefix as returned from the fetch.
		id.PrefixLenBits = pxLen * depthQuantum
		if _, ok := s.subtrees[pxKey]; !ok {
			want[pxKey] = &id
		}
	}

	// There might be nothing to do so don't make a read request for zero subtrees if so
	if len(want) == 0 {
		return nil
	}

	list := make([]storage.NodeID, 0, len(want))
	for _, v := range want {
		list = append(list, *v)
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
		s.subtrees[string(t.Prefix)] = t
		delete(want, string(t.Prefix))
	}

	// We might not have got all the subtrees we requested, if they don't already exist.
	// Create empty subtrees for anything left over. As an additional sanity check we refuse
	// to overwrite anything already in the cache as we determined above that these subtrees
	// should not exist in the subtree cache map.
	for _, id := range want {
		prefixLen := id.PrefixLenBits / depthQuantum
		px := id.Path[:prefixLen]
		pxKey := string(px)
		_, exists := s.subtrees[pxKey]
		if exists {
			return fmt.Errorf("preload tried to clobber existing subtree for: %v", *id)
		}
		s.subtrees[pxKey] = s.newEmptySubtree(*id, px)
	}

	return nil
}

// GetNodes returns the requested nodes, calling the getSubtrees function if
// they are not already cached.
func (s *SubtreeCache) GetNodes(ids []storage.NodeID, getSubtrees GetSubtreesFunc) ([]storage.Node, error) {
	glog.V(2).Infof("cache: GetNodes(count=%d)", len(ids))
	if glog.V(3) {
		for _, n := range ids {
			glog.Infof("  cache: GetNodes(path=%x, prefixLen=%d)", n.Path, n.PrefixLenBits)
		}
	}
	if err := s.preload(ids, getSubtrees); err != nil {
		return nil, err
	}

	ret := make([]storage.Node, 0, len(ids))
	for _, id := range ids {
		h, err := s.getNodeHash(
			id,
			func(n storage.NodeID) (*storagepb.SubtreeProto, error) {
				// This should never happen - we should've already read all the data we
				// need above, in Preload()
				glog.Warningf("Unexpectedly reading from within getNodeHash(): %s", n.String())
				ret, err := getSubtrees([]storage.NodeID{n})
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
			ret = append(ret, storage.Node{
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

// getNodeHash returns a single node hash from the cache.
func (s *SubtreeCache) getNodeHash(id storage.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) {", id.Path, id.PrefixLenBits)
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	data, err := s.getNodeHashUnderLock(id, getSubtree)
	if glog.V(3) {
		glog.Infof("cache: getNodeHash(path=%x, prefixLen=%d) => %x, %v }", id.Path, id.PrefixLenBits, data, err)
	}
	return data, err
}

// getNodeHashUnderLock must be called with s.mutex locked.
func (s *SubtreeCache) getNodeHashUnderLock(id storage.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	px, sx := s.splitNodeID(id)
	prefixKey := string(px)
	c := s.subtrees[prefixKey]
	if c == nil {
		glog.V(2).Infof("Cache miss for %x so we'll try to fetch from storage", prefixKey)
		// Cache miss, so we'll try to fetch from storage.
		subID := id
		subID.PrefixLenBits = len(px) * depthQuantum // this won't work if depthQuantum changes
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

		s.subtrees[prefixKey] = c
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
		glog.Infof("getNodeHashUnderLock(%x | %x): %x", prefixKey, b, nh)
	}
	if nh == nil {
		return nil, nil
	}
	return nh, nil
}

// SetNodeHash sets a node hash in the cache.
func (s *SubtreeCache) SetNodeHash(id storage.NodeID, h []byte, getSubtree GetSubtreeFunc) error {
	if glog.V(3) {
		glog.Infof("cache: SetNodeHash(%x, %d)=%x", id.Path, id.PrefixLenBits, h)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	px, sx := s.splitNodeID(id)
	prefixKey := string(px)
	c := s.subtrees[prefixKey]
	if c == nil {
		// TODO(al): This is ok, IFF *all* leaves in the subtree are being set,
		// verify that this is the case when it happens.
		// For now, just read from storage if we don't already have it.
		glog.V(1).Infof("attempting to write to unread subtree for %v, reading now", id.String())
		// We hold the lock so can call this directly:
		_, err := s.getNodeHashUnderLock(id, getSubtree)
		if err != nil {
			return err
		}
		// There must be a subtree present in the cache now, even if storage didn't have anything for us.
		c = s.subtrees[prefixKey]
		if c == nil {
			return fmt.Errorf("internal error, subtree cache for %v is nil after a read attempt", id.String())
		}
	}
	if c.Prefix == nil {
		return fmt.Errorf("nil prefix for %v (key %v)", id.String(), prefixKey)
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
	s.dirtyPrefixes[prefixKey] = true
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
func (s *SubtreeCache) Flush(setSubtrees SetSubtreesFunc) error {
	glog.V(1).Info("cache: Flush")
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	treesToWrite := make([]*storagepb.SubtreeProto, 0, len(s.dirtyPrefixes))
	for k, v := range s.subtrees {
		if s.dirtyPrefixes[k] {
			bk := []byte(k)
			if !bytes.Equal(bk, v.Prefix) {
				return fmt.Errorf("inconsistent cache: prefix key is %v, but cached object claims %v", bk, v.Prefix)
			}
			// TODO(al): Do actually write this one once we're storing the updated
			// subtree root value here during tree update calculations.
			v.RootHash = nil

			if len(v.Leaves) > 0 {
				// prepare internal nodes ready for the write (tree type specific)
				if err := s.prepare(v); err != nil {
					return err
				}
				treesToWrite = append(treesToWrite, v)
			}
		}
	}
	if len(treesToWrite) == 0 {
		return nil
	}
	err := setSubtrees(treesToWrite)
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

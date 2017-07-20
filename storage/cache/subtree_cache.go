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
	"fmt"
	"math/big"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
)

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
			sInfo = append(sInfo, stratumInfo{pb, sDepth})
			t += depthQuantum
		}
	}
	// TODO(al): This needs to be passed in, particularly for Map use cases where
	// we need to know it matches the number of bits in the chosen hash function.
	if got, want := t, maxTreeDepth; got != want {
		panic(fmt.Errorf("strata indicate tree of depth %d, but expected %d", got, want))
	}

	return SubtreeCache{
		stratumInfo:   sInfo,
		subtrees:      make(map[string]*storagepb.SubtreeProto),
		dirtyPrefixes: make(map[string]bool),
		mutex:         new(sync.RWMutex),
		populate:      populateSubtree,
		prepare:       prepareSubtreeWrite,
	}
}

func (s *SubtreeCache) stratumInfoForPrefixLength(numBits int) stratumInfo {
	return s.stratumInfo[numBits/depthQuantum]
}

// splitNodeID breaks a NodeID out into its prefix and suffix parts.
// unless ID is 0 bits long, Suffix must always contain at least one bit.
func (s *SubtreeCache) splitNodeID(id storage.NodeID) ([]byte, storage.Suffix) {
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
		px, _ := s.splitNodeID(id)
		pxKey := string(px)
		_, ok := s.subtrees[pxKey]
		// TODO(al): fix for non-uniform strata
		id.PrefixLenBits = len(px) * depthQuantum
		if !ok {
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
	for _, t := range subtrees {
		s.populate(t)
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
	if err := s.preload(ids, getSubtrees); err != nil {
		return nil, err
	}

	ret := make([]storage.Node, 0, len(ids))
	for _, id := range ids {
		h, err := s.GetNodeHash(
			id,
			func(n storage.NodeID) (*storagepb.SubtreeProto, error) {
				// This should never happen - we should've already read all the data we
				// need above, in Preload()
				glog.Warningf("Unexpectedly reading from within GetNodeHash(): %s", n.String())
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
	return ret, nil
}

// GetNodeHash returns a single node hash from the cache.
func (s *SubtreeCache) GetNodeHash(id storage.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.getNodeHashUnderLock(id, getSubtree)
}

// getNodeHashUnderLock must be called with s.mutex locked.
func (s *SubtreeCache) getNodeHashUnderLock(id storage.NodeID, getSubtree GetSubtreeFunc) ([]byte, error) {
	px, sx := s.splitNodeID(id)
	prefixKey := string(px)
	c := s.subtrees[prefixKey]
	if c == nil {
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
			panic(fmt.Errorf("GetNodeHash nil prefix on %v for id %v with px %#v", c, id.String(), px))
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
	if int32(sx.Bits) == c.Depth {
		nh = c.Leaves[sfxKey]
	} else {
		nh = c.InternalNodes[sfxKey]
	}
	if nh == nil {
		return nil, nil
	}
	return nh, nil
}

// SetNodeHash sets a node hash in the cache.
func (s *SubtreeCache) SetNodeHash(id storage.NodeID, h []byte, getSubtree GetSubtreeFunc) error {
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
	s.dirtyPrefixes[prefixKey] = true
	// Determine whether we're being asked to store a leaf node, or an internal
	// node, and store it accordingly.
	sfxKey := sx.String()
	if int32(sx.Bits) == c.Depth {
		c.Leaves[sfxKey] = h
	} else {
		c.InternalNodes[sfxKey] = h
	}
	return nil
}

// Flush causes the cache to write all dirty Subtrees back to storage.
func (s *SubtreeCache) Flush(setSubtrees SetSubtreesFunc) error {
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
	return setSubtrees(treesToWrite)
}

func (s *SubtreeCache) newEmptySubtree(id storage.NodeID, px []byte) *storagepb.SubtreeProto {
	sInfo := s.stratumInfoForPrefixLength(id.PrefixLenBits)
	glog.V(1).Infof("Creating new empty subtree for %v, with depth %d", px, sInfo.depth)
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

// PopulateMapSubtreeNodes re-creates Map subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses HStar2 to repopulate internal nodes.
func PopulateMapSubtreeNodes(treeID int64, hasher hashers.MapHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = make(map[string][]byte)
		rootID := storage.NewNodeIDFromHash(st.Prefix)
		leaves := make([]merkle.HStar2LeafHash, 0, len(st.Leaves))
		for k64, v := range st.Leaves {
			k, err := base64.StdEncoding.DecodeString(k64)
			if err != nil {
				return err
			}
			if k[0]%depthQuantum != 0 {
				return fmt.Errorf("unexpected non-leaf suffix found: %x", k)
			}
			leaves = append(leaves, merkle.HStar2LeafHash{
				LeafHash: v,
				Index:    big.NewInt(int64(k[1])),
			})
		}
		hs2 := merkle.NewHStar2(treeID, hasher)
		offset := hasher.BitLen() - rootID.PrefixLenBits - int(st.Depth)
		root, err := hs2.HStar2Nodes(int(st.Depth), offset, leaves,
			func(depth int, index *big.Int) ([]byte, error) {
				return nil, nil
			},
			func(depth int, index *big.Int, h []byte) error {
				nodeID := storage.NewNodeIDFromRelativeBigInt(st.Prefix, depth, index, hasher.BitLen())
				_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
				sfxKey := sfx.String()
				st.InternalNodes[sfxKey] = h
				return nil
			})
		if err != nil {
			return err
		}
		st.RootHash = root
		return err
	}
}

// PopulateLogSubtreeNodes re-creates a Log subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses the CompactMerkleTree to repopulate internal nodes, and so will
// handle imperfect (but left-hand dense) subtrees. Note that we only rebuild internal
// nodes when the subtree is fully populated. For an explanation of why see the comments
// below for PrepareLogSubtreeWrite.
func PopulateLogSubtreeNodes(hasher hashers.LogHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		cmt := merkle.NewCompactMerkleTree(hasher)
		if st.Depth < 1 {
			return fmt.Errorf("populate log subtree with invalid depth: %d", st.Depth)
		}
		// maxLeaves is the number of leaves that fully populates a subtree of the depth we are
		// working with.
		maxLeaves := 1 << uint(st.Depth)

		// If the subtree is fully populated then the internal node map is expected to be nil but in
		// case it isn't we recreate it as we're about to rebuild the contents. We'll check
		// below that the number of nodes is what we expected to have.
		if st.InternalNodes == nil || len(st.Leaves) == maxLeaves {
			st.InternalNodes = make(map[string][]byte)
		}

		// We need to update the subtree root hash regardless of whether it's fully populated
		for leafIndex := int64(0); leafIndex < int64(len(st.Leaves)); leafIndex++ {
			nodeID := storage.NewNodeIDFromPrefix(st.Prefix, logStrataDepth, leafIndex, logStrataDepth, maxLogDepth)
			_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
			sfxKey := sfx.String()
			h := st.Leaves[sfxKey]
			if h == nil {
				return fmt.Errorf("unexpectedly got nil for subtree leaf suffix %s", sfx)
			}
			seq, err := cmt.AddLeafHash(h, func(height int, index int64, h []byte) error {
				if height == logStrataDepth && index == 0 {
					// no space for the root in the node cache
					return nil
				}

				subDepth := logStrataDepth - height
				nodeID := storage.NewNodeIDFromPrefix(st.Prefix, subDepth, index, logStrataDepth, maxLogDepth)
				_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
				sfxKey := sfx.String()
				// Don't put leaves into the internal map and only update if we're rebuilding internal
				// nodes. If the subtree was saved with internal nodes then we don't touch the map.
				if height > 0 && len(st.Leaves) == maxLeaves {
					st.InternalNodes[sfxKey] = h
				}
				return nil
			})
			if err != nil {
				return err
			}
			if got, expected := seq, leafIndex; got != expected {
				return fmt.Errorf("got seq of %d, but expected %d", got, expected)
			}
		}
		st.RootHash = cmt.CurrentRoot()

		// Additional check - after population we should have the same number of internal nodes
		// as before the subtree was written to storage. Either because they were loaded from
		// storage or just rebuilt above.
		if got, want := uint32(len(st.InternalNodes)), st.InternalNodeCount; got != want {
			// TODO(Martin2112): Possibly replace this with stronger checks on the data in
			// subtrees on disk so we can detect corruption.
			return fmt.Errorf("log repop got: %d internal nodes, want: %d", got, want)
		}

		return nil
	}
}

// PrepareMapSubtreeWrite prepares a map subtree for writing. For maps the internal
// nodes are never written to storage and are thus always cleared.
func PrepareMapSubtreeWrite() storage.PrepareSubtreeWriteFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = nil
		// We don't check the node count for map subtrees but ensure it's zero for consistency
		st.InternalNodeCount = 0
		return nil
	}
}

// PrepareLogSubtreeWrite prepares a log subtree for writing. If the subtree is fully
// populated the internal nodes are cleared. Otherwise they are written.
//
// To see why this is necessary consider the case where a tree has a single full subtree
// and then an additional leaf is added.
//
// This causes an extra level to be added to the tree with an internal node that is a hash
// of the root of the left full subtree and the new leaf. Note that the leaves remain at
// level zero in the overall tree coordinate space but they are now in a lower subtree stratum
// than they were before the last node was added as the tree has grown above them.
//
// Thus in the case just discussed the internal nodes cannot be correctly reconstructed
// in isolation when the tree is reloaded because of the dependency on another subtree.
//
// Fully populated subtrees don't have this problem because by definition they can only
// contain internal nodes built from their own contents.
func PrepareLogSubtreeWrite() storage.PrepareSubtreeWriteFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodeCount = uint32(len(st.InternalNodes))
		if st.Depth < 1 {
			return fmt.Errorf("prepare subtree for log write invalid depth: %d", st.Depth)
		}
		maxLeaves := 1 << uint(st.Depth)
		// If the subtree is fully populated we can safely clear the internal nodes
		if len(st.Leaves) == maxLeaves {
			st.InternalNodes = nil
		}
		return nil
	}
}

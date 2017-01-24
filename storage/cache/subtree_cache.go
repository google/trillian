package cache

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle"
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

// SubtreeCache provides a caching access to Subtree storage.
type SubtreeCache struct {
	// prefixLengths contains the strata prefix sizes for each multiple-of-8 tree
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

	populateSubtree storage.PopulateSubtreeFunc
}

// Suffix represents the tail of a NodeID, indexing into the Subtree which
// corresponds to the prefix of the NodeID.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	bits byte
	// path is the suffix itself.
	path []byte
}

func (s Suffix) serialize() string {
	r := make([]byte, 1, 1+(len(s.path)))
	r[0] = s.bits
	r = append(r, s.path...)
	return base64.StdEncoding.EncodeToString(r)
}

// NewSubtreeCache returns a newly intialised cache ready for use.
// populateSubtree is a function which knows how to populate a subtree's
// internal nodes given its leaves, and will be called for each subtree loaded
// from storage.
// TODO(al): consider supporting different sized subtrees - for now everything's subtrees of 8 levels.
func NewSubtreeCache(strataDepths []int, populateSubtree storage.PopulateSubtreeFunc) SubtreeCache {
	// TODO(al): pass this in
	maxTreeDepth := 256
	// Precalculate strata information based on the passed in strata depths:
	sInfo := make([]stratumInfo, 0, maxTreeDepth/8)
	t := 0
	for _, sDepth := range strataDepths {
		// Verify the stratum depth makes sense:
		if sDepth <= 0 {
			panic(fmt.Errorf("got invalid strata depth of %d: can't be <= 0", sDepth))
		}
		if sDepth%8 != 0 {
			panic(fmt.Errorf("got strata depth of %d, must be a multiple of 8", sDepth))
		}

		pb := t / 8
		for i := 0; i < sDepth; i += 8 {
			sInfo = append(sInfo, stratumInfo{pb, sDepth})
			t += 8
		}
	}
	// TODO(al): This needs to be passed in, particularly for Map use cases where
	// we need to know it matches the number of bits in the chosen hash function.
	if got, want := t, maxTreeDepth; got != want {
		panic(fmt.Errorf("strata indicate tree of depth %d, but expected %d", got, want))
	}

	return SubtreeCache{
		stratumInfo:     sInfo,
		subtrees:        make(map[string]*storagepb.SubtreeProto),
		dirtyPrefixes:   make(map[string]bool),
		mutex:           new(sync.RWMutex),
		populateSubtree: populateSubtree,
	}
}

func (s *SubtreeCache) stratumInfoForPrefixLength(numBits int) stratumInfo {
	return s.stratumInfo[numBits/8]
}

// splitNodeID breaks a NodeID out into its prefix and suffix parts.
// unless ID is 0 bits long, Suffix must always contain at least one bit.
func (s *SubtreeCache) splitNodeID(id storage.NodeID) ([]byte, Suffix) {
	if id.PrefixLenBits == 0 {
		return []byte{}, Suffix{bits: 0, path: []byte{0}}
	}
	a := make([]byte, len(id.Path))
	copy(a, id.Path)
	sInfo := s.stratumInfoForPrefixLength(id.PrefixLenBits - 1)
	prefixSplit := sInfo.prefixBytes
	sfx := Suffix{
		bits: byte((id.PrefixLenBits-1)%sInfo.depth) + 1,
		path: a[prefixSplit : prefixSplit+sInfo.depth/8],
	}
	maskIndex := int((sfx.bits - 1) / 8)
	maskLowBits := (sfx.bits-1)%8 + 1
	sfx.path[maskIndex] &= ((0x01 << maskLowBits) - 1) << uint(8-maskLowBits)

	return a[:prefixSplit], sfx
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
		id.PrefixLenBits = len(px) * 8
		if !ok {
			want[pxKey] = &id
		}
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
		s.populateSubtree(t)
		s.subtrees[string(t.Prefix)] = t
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
				glog.Warning("Unexpectedly reading from within GetNodeHash()")
				ret, err := getSubtrees([]storage.NodeID{n})
				if err != nil || len(ret) == 0 {
					return nil, err
				}
				if n := len(ret); n > 1 {
					return nil, fmt.Errorf("got %d trees, wanted 1", n)
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
		subID.PrefixLenBits = len(px) * 8
		var err error
		c, err = getSubtree(subID)
		if err != nil {
			return nil, err
		}
		sInfo := s.stratumInfoForPrefixLength(subID.PrefixLenBits)
		if c == nil {
			glog.V(1).Infof("Creating new empty subtree for %v, with depth %d", px, sInfo.depth)
			// storage didn't have one for us, so we'll store an empty proto here
			// incase we try to update it later on (we won't flush it back to
			// storage unless it's been written to.)
			c = &storagepb.SubtreeProto{
				Prefix:        px,
				Depth:         int32(sInfo.depth),
				Leaves:        make(map[string][]byte),
				InternalNodes: make(map[string][]byte),
			}
		} else {
			if err := s.populateSubtree(c); err != nil {
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
	// we can easily dump (and later reconstruct) the internal nodes.
	// Since the subtrees are fixed to a depth of 8, any suffix with 8
	// significant bits must be a leaf hash.
	if int32(sx.bits) == c.Depth {
		nh = c.Leaves[sx.serialize()]
	} else {
		nh = c.InternalNodes[sx.serialize()]
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
		panic(fmt.Errorf("nil prefix for %v (key %v)", id.String(), prefixKey))
	}
	s.dirtyPrefixes[prefixKey] = true
	// Determine whether we're being asked to store a leaf node, or an internal
	// node, and store it accordingly.
	if int32(sx.bits) == c.Depth {
		c.Leaves[sx.serialize()] = h
	} else {
		c.InternalNodes[sx.serialize()] = h
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
				// clear the internal node cache; we don't want to write that.
				v.InternalNodes = nil
				treesToWrite = append(treesToWrite, v)
			}
		}
	}
	if len(treesToWrite) == 0 {
		return nil
	}
	return setSubtrees(treesToWrite)
}

// makeSuffixKey creates a suffix key for indexing into the subtree's Leaves and
// InternalNodes maps.
func makeSuffixKey(depth int, index int64) (string, error) {
	if depth < 0 {
		return "", fmt.Errorf("invalid negative depth of %d", depth)
	}
	if index < 0 {
		return "", fmt.Errorf("invalid negative index %d", index)
	}
	sfx := Suffix{byte(depth), []byte{byte(index)}}
	return sfx.serialize(), nil
}

// PopulateMapSubtreeNodes re-creates Map subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses HStar2 to repopulate internal nodes.
func PopulateMapSubtreeNodes(treeHasher merkle.TreeHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = make(map[string][]byte)
		rootID := storage.NewNodeIDFromHash(st.Prefix)
		fullTreeDepth := treeHasher.Size() * 8
		leaves := make([]merkle.HStar2LeafHash, 0, len(st.Leaves))
		for k64, v := range st.Leaves {
			k, err := base64.StdEncoding.DecodeString(k64)
			if err != nil {
				return err
			}
			if k[0]%8 != 0 {
				return fmt.Errorf("unexpected non-leaf suffix found: %x", k)
			}
			leaves = append(leaves, merkle.HStar2LeafHash{
				LeafHash: v,
				Index:    big.NewInt(int64(k[1])),
			})
		}
		hs2 := merkle.NewHStar2(treeHasher)
		offset := fullTreeDepth - rootID.PrefixLenBits - int(st.Depth)
		root, err := hs2.HStar2Nodes(int(st.Depth), offset, leaves,
			func(depth int, index *big.Int) ([]byte, error) {
				return nil, nil
			},
			func(depth int, index *big.Int, h []byte) error {
				i := index.Int64()
				sfx, err := makeSuffixKey(depth, i)
				if err != nil {
					return err
				}
				st.InternalNodes[sfx] = h
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
// handle imperfect (but left-hand dense) subtrees.
func PopulateLogSubtreeNodes(treeHasher merkle.TreeHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = make(map[string][]byte)
		cmt := merkle.NewCompactMerkleTree(treeHasher)
		for leafIndex := int64(0); leafIndex < int64(len(st.Leaves)); leafIndex++ {
			sfx, err := makeSuffixKey(8, leafIndex)
			if err != nil {
				return err
			}
			h := st.Leaves[sfx]
			if h == nil {
				return fmt.Errorf("unexpectedly got nil for subtree leaf suffix %s", sfx)
			}
			seq := cmt.AddLeafHash(h, func(depth int, index int64, h []byte) {
				if depth == 8 && index == 0 {
					// no space for the root in the node cache
					return
				}
				key, err := makeSuffixKey(8-depth, index<<uint(depth))
				if err != nil {
					// TODO(al): Don't panic Mr. Mainwaring.
					panic(err)
				}
				if depth > 0 {
					st.InternalNodes[key] = h
				}
			})
			if got, expected := seq, leafIndex; got != expected {
				return fmt.Errorf("got seq of %d, but expected %d", got, expected)
			}
		}
		st.RootHash = cmt.CurrentRoot()
		return nil
	}
}

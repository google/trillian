package cache

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
)

// GetSubtreeFunc describes a function which can return a Subtree from storage.
type GetSubtreeFunc func(id storage.NodeID) (*storage.SubtreeProto, error)

// SetSubtreeFunc describes a function which can store a Subtree into storage.
type SetSubtreeFunc func(s *storage.SubtreeProto) error

// SubtreeCache provides a caching access to Subtree storage.
type SubtreeCache struct {
	// subtrees contains the Subtree data read from storage, and is updated by
	// calls to SetNodeHash.
	subtrees map[string]*storage.SubtreeProto
	// dirtyPrefixes keeps track of all Subtrees which need to be written back
	// to storage.
	dirtyPrefixes map[string]bool
	// mutex guards access to the maps above.
	mutex *sync.RWMutex

	populateSubtree storage.PopulateSubtreeFunc
}

// Suffix represents the tail of of a NodeID, indexing into the Subtree which
// corresponds to the prefix of the NodeID.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	bits byte
	// path is the suffix itself.
	path []byte
}

func (s Suffix) serialize() string {
	r := make([]byte, 1, len(s.path)+1)
	r[0] = s.bits
	r = append(r, s.path...)
	return base64.StdEncoding.EncodeToString(r)
}

const (
	// strataDepth is the depth of Subtree.
	strataDepth = 8
)

// NewSubtreeCache returns a newly intialised cache ready for use.
// populateSubtree is a function which knows how to populate a subtree's
// internal nodes given its leaves, and will be called for each subtree loaded
// from storage.
// TODO(al): consider supporting different sized subtrees - for now everything's subtrees of 8 levels.
func NewSubtreeCache(populateSubtree storage.PopulateSubtreeFunc) SubtreeCache {
	return SubtreeCache{
		subtrees:        make(map[string]*storage.SubtreeProto),
		dirtyPrefixes:   make(map[string]bool),
		mutex:           new(sync.RWMutex),
		populateSubtree: populateSubtree,
	}
}

// splitNodeID breaks a NodeID out into its prefix and suffix parts.
// unless ID is 0 bits long, Suffix must always contain at least one bit.
func splitNodeID(id storage.NodeID) ([]byte, Suffix) {
	if id.PrefixLenBits == 0 {
		return []byte{}, Suffix{bits: 0, path: []byte{0}}
	}
	prefixSplit := (id.PrefixLenBits - 1) / strataDepth
	suffixEnd := (id.PrefixLenBits-1)/8 + 1
	s := Suffix{
		bits: byte((id.PrefixLenBits-1)%strataDepth) + 1,
		path: make([]byte, suffixEnd-prefixSplit),
	}
	// TODO(al): is all this copying actually necessary?
	copy(s.path, id.Path[prefixSplit:suffixEnd])
	if id.PrefixLenBits%8 != 0 {
		suffixMask := (byte(0x1<<uint((id.PrefixLenBits%8))) - 1) << uint(8-id.PrefixLenBits%8)
		s.path[len(s.path)-1] &= suffixMask
	}

	// TODO(al): is all this copying actually necessary?
	r := make([]byte, prefixSplit)
	copy(r, id.Path[:prefixSplit])
	return r, s
}

// GetNodeHash retrieves the previously written hash and corresponding tree
// revision for the given node ID.
func (s *SubtreeCache) GetNodeHash(id storage.NodeID, getSubtree GetSubtreeFunc) (trillian.Hash, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.getNodeHashUnderLock(id, getSubtree)
}

// getNodeHashUnderLock must be called with s.mutex locked.
func (s *SubtreeCache) getNodeHashUnderLock(id storage.NodeID, getSubtree GetSubtreeFunc) (trillian.Hash, error) {
	px, sx := splitNodeID(id)
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
		if c == nil {
			// storage didn't have one for us, so we'll store an empty proto here
			// incase we try to update it later on (we won't flush it back to
			// storage unless it's been written to.)
			c = &storage.SubtreeProto{
				Prefix:        px,
				Depth:         strataDepth,
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
	var nh trillian.Hash
	// Look up the hash in the appropriate map
	if sx.bits == 8 {
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
func (s *SubtreeCache) SetNodeHash(id storage.NodeID, h trillian.Hash, getSubtree GetSubtreeFunc) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	px, sx := splitNodeID(id)
	prefixKey := string(px)
	c := s.subtrees[prefixKey]
	if c == nil {
		// TODO(al): This is ok, IFF *all* leaves in the subtree are being set,
		// verify that this is the case when it happens.
		// For now, just read from storage if we don't already have it.
		glog.Infof("attempting to write to unread subtree for %v, reading now", id.String())
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
	if sx.bits == 8 {
		c.Leaves[sx.serialize()] = h
	} else {
		c.InternalNodes[sx.serialize()] = h
	}
	return nil
}

// Flush causes the cache to write all dirty Subtrees back to storage.
func (s *SubtreeCache) Flush(setSubtree SetSubtreeFunc) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for k, v := range s.subtrees {
		if s.dirtyPrefixes[k] {
			bk := []byte(k)
			if !bytes.Equal(bk, v.Prefix) {
				return fmt.Errorf("inconsistent cache: prefix key is %v, but cached object claims %v", bk, v.Prefix)
			}
			if len(v.Leaves) > 0 {
				// clear the internal node cache; we don't want to write that.
				v.InternalNodes = nil
				if err := setSubtree(v); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// makeSuffixKey creates a suffix key for indexing into the subtree's Leaves and
// InternalNodes maps.
func makeSuffixKey(depth int, index int64) (string, error) {
	// TODO(al): only supports 8 bit subtree sizes currently
	if depth > 8 || depth < 0 {
		return "", fmt.Errorf("found depth of %d, but we only support positive depths of up to and including 8 currently", depth)
	}
	if index > 255 || index < 0 {
		return "", fmt.Errorf("got unsupported index of %d, 0 <= index < 256", index)
	}
	sfx := Suffix{byte(depth), []byte{byte(index)}}
	return sfx.serialize(), nil
}

// PopulateMapSubtreeNodes re-creates Map subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses HStar2 to repopulate internal nodes.
func PopulateMapSubtreeNodes(treeHasher merkle.TreeHasher) storage.PopulateSubtreeFunc {
	return func(st *storage.SubtreeProto) error {
		st.InternalNodes = make(map[string][]byte)
		rootID := storage.NewNodeIDFromHash(st.Prefix)
		fullTreeDepth := treeHasher.Size() * 8
		leaves := make([]merkle.HStar2LeafHash, 0, len(st.Leaves))
		for k64, v := range st.Leaves {
			k, err := base64.StdEncoding.DecodeString(k64)
			if err != nil {
				return err
			}
			if k[0] != 8 {
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
			func(depth int, index *big.Int) (trillian.Hash, error) {
				return nil, nil
			},
			func(depth int, index *big.Int, h trillian.Hash) error {
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
	return func(st *storage.SubtreeProto) error {
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
			seq := cmt.AddLeafHash(h, func(depth int, index int64, h trillian.Hash) {
				if depth == 8 && index == 0 {
					// no space for the root in the node cache
					return
				}
				key, err := makeSuffixKey(depth, index)
				if err != nil {
					// TODO(al): Don't panic Mr. Mainwaring.
					panic(err)
				}
				st.InternalNodes[key] = h
			})
			if got, expected := seq, leafIndex; got != expected {
				return fmt.Errorf("got seq of %d, but expected %d", got, expected)
			}
		}
		st.RootHash = cmt.CurrentRoot()
		return nil
	}
}

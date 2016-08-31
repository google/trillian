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

	// treeHasher is used for re-creating subtree nodes from the stored subtree
	// leaves.
	treeHasher merkle.TreeHasher
}

// Suffix represents the tail of of a NodeID, indexing into the Subtree which
// corresponds to the prefix of the NodeID.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	bits byte
	// path is the suffix itself.
	path []byte
}

func (s *Suffix) serialize() string {
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
// TODO(al): consider supporting different sized subtrees - for now everything's subtrees of 8 levels.
func NewSubtreeCache(t merkle.TreeHasher) SubtreeCache {
	return SubtreeCache{
		subtrees:      make(map[string]*storage.SubtreeProto),
		dirtyPrefixes: make(map[string]bool),
		mutex:         new(sync.RWMutex),
		treeHasher:    t,
	}
}

// splitNodeID breaks a NodeID out into its prefix and suffix parts.
// unless ID is 0 bits long, Suffix must always contain at least one bit.
func splitNodeID(id storage.NodeID) ([]byte, Suffix) {
	if id.PrefixLenBits == 0 {
		return []byte{}, Suffix{bits: 0, path: []byte{}}
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
				Prefix: px,
				Depth:  strataDepth,
				Leaves: make(map[string][]byte),
			}
		}
		// Whether from storage or freshly created, we'll need an internal nodes cache map:
		c.InternalNodes = make(map[string][]byte)
		if c.Prefix == nil {
			panic(fmt.Errorf("GetNodeHash nil prefix on %v for id %v with px %#v", c, id.String(), px))
		}

		s.subtrees[prefixKey] = c
	}

	// finally look for the particular node within the subtree so we can return
	// the hash & revision.
	var nh trillian.Hash
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

// makeSuffix creates a suffix key for indexing into the subtree's Leaves and
// InternalNodes maps.
func makeSuffix(depth int, index *big.Int) ([]byte, error) {
	// TODO(al): only supports 8 bit subtree sizes currently
	sfx := make([]byte, 2)
	if depth >= 8 {
		return nil, fmt.Errorf("found depth of %d, but we only support depths of 8 currently", depth)
	}
	sfx[0] = byte(depth)
	if index.BitLen() > 0 {
		sfx[1] = index.Bytes()[0]
	}
	return sfx, nil
}

// populateSubtreeNodes re-creates the subtree's InternalNodes from the subtree
// Leaves map.
func (s *SubtreeCache) populateSubtreeNodes(st *storage.SubtreeProto) error {
	st.InternalNodes = make(map[string][]byte)
	rootID := storage.NewNodeIDFromHash(st.Prefix)
	fullTreeDepth := s.treeHasher.Size() * 8
	leaves := make([]merkle.HStar2LeafHash, 0, len(st.Leaves))
	for k, v := range st.Leaves {
		if k[0] != 8 {
			return fmt.Errorf("unexpected non-leaf suffix found: %v", k)
		}
		leaves = append(leaves, merkle.HStar2LeafHash{
			LeafHash: v,
			Index:    big.NewInt(int64(k[1])),
		})
	}
	hs2 := merkle.NewHStar2(s.treeHasher)
	offset := fullTreeDepth - rootID.PrefixLenBits - int(st.Depth)
	_, err := hs2.HStar2Nodes(int(st.Depth), offset, leaves,
		func(depth int, index *big.Int) (trillian.Hash, error) {
			return nil, nil
		},
		func(depth int, index *big.Int, h trillian.Hash) error {
			sfx, err := makeSuffix(depth, index)
			if err != nil {
				return err
			}
			st.InternalNodes[string(sfx)] = h
			return nil
		})
	// TODO(al): Store a Subtree's root hash too, so we can verify we've
	// recalculated the same hash here.
	return err
}

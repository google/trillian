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

package testonly

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
)

// ErrInvariant indicates that an invariant check failed, with details in msg.
type ErrInvariant struct {
	msg string
}

func (e ErrInvariant) Error() string {
	return fmt.Sprintf("Invariant check failed: %v", e.msg)
}

// MapContents is a complete copy of the map's contents at a particular revision.
type MapContents struct {
	Rev  int64
	data map[mapKey]string
}

type mapKey [sha256.Size]byte

func (m *MapContents) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("rev: %d data:", m.Rev))
	for k, v := range m.data {
		buf.WriteString(fmt.Sprintf(" k: %x v: %q,", k, v))
	}
	return buf.String()
}

// Empty indicates if the contents are empty.
func (m *MapContents) Empty() bool {
	if m == nil {
		return true
	}
	return len(m.data) == 0
}

// PickKey randomly selects a key that already exists in a given copy of the
// map's contents. Assumes that the copy is non-empty.
func (m *MapContents) PickKey(prng *rand.Rand) []byte {
	if m.Empty() {
		panic("internal error: can't pick a key, map data is empty!")
	}

	choice := prng.Intn(len(m.data))
	// Need sorted keys for reproduceability.
	keys := make([]mapKey, 0)
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) == -1
	})
	return keys[choice][:]
}

// CheckContents compares information returned from the Map against a local copy
// of the map's contents.
func (m *MapContents) CheckContents(leafInclusions []*trillian.MapLeafInclusion, extraSize uint) error {
	for _, inc := range leafInclusions {
		leaf := inc.Leaf
		var key mapKey
		copy(key[:], leaf.Index)
		value, ok := m.data[key]
		if ok {
			if string(leaf.LeafValue) != value {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want %q", key, leaf.LeafValue, value)
			}
			if want := ExtraDataForValue(leaf.LeafValue, extraSize); !bytes.Equal(leaf.ExtraData, want) {
				return fmt.Errorf("got leaf[%v].ExtraData=%q, want %q", key, leaf.ExtraData, want)
			}
		} else {
			if len(leaf.LeafValue) > 0 {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want not-present", key, leaf.LeafValue)
			}
		}
	}
	return nil
}

// UpdatedWith returns a new MapContents object that has been updated to include the
// given leaves and revision.  A nil receiver object is allowed.
func (m *MapContents) UpdatedWith(rev uint64, leaves []*trillian.MapLeaf) *MapContents {
	// Start from previous map contents
	result := MapContents{Rev: int64(rev), data: make(map[mapKey]string)}
	if m != nil {
		for k, v := range m.data {
			result.data[k] = v
		}
	}
	// Update with given leaves
	for _, leaf := range leaves {
		var k mapKey
		copy(k[:], leaf.Index)
		result.data[k] = string(leaf.LeafValue)
	}

	return &result
}

type bitString struct {
	data   [sha256.Size]byte
	bitLen int
}

var topBitsMask = map[int]byte{0: 0x00, 1: 0x80, 2: 0xc0, 3: 0xe0, 4: 0xf0, 5: 0xf8, 6: 0xfc, 7: 0xfe}

func truncateToNBits(in bitString, bitLen int) bitString {
	if bitLen > in.bitLen {
		panic(fmt.Sprintf("truncating %d bits to %d bits!", in.bitLen, bitLen))
	}
	// Wipe bits so hashing/comparison works.
	byteCount := (bitLen + 7) / 8 // includes partial bytes
	result := bitString{bitLen: bitLen}
	copy(result.data[:], in.data[:byteCount])

	topBitsToKeep := (bitLen % 8)

	if topBitsToKeep > 0 {
		mask := topBitsMask[topBitsToKeep]
		result.data[byteCount-1] &= mask
	}
	return result
}

func extendByABit(in bitString, bitValue int) bitString {
	result := bitString{data: in.data, bitLen: in.bitLen + 1}
	index := in.bitLen / 8
	mask := byte(1 << (7 - uint(in.bitLen%8)))
	if bitValue == 0 {
		mask = ^mask
		result.data[index] &= mask
	} else {
		result.data[index] |= mask
	}
	return result
}

// RootHash performs a slow and simple calculation of the root hash for a sparse
// Merkle tree with the given leaf contents.
func (m *MapContents) RootHash(treeID int64, hasher hashers.MapHasher) ([]byte, error) {
	// Watch out for completely empty trees
	if m == nil || len(m.data) == 0 {
		return hasher.HashEmpty(treeID, []byte{}, hasher.BitLen()), nil
	}
	// First do the leaves, as they're special.
	curBitLen := hasher.BitLen()
	curHeight := 0
	curHashes := make(map[bitString][]byte)
	for k, v := range m.data {
		prefixKey := bitString{bitLen: curBitLen}
		copy(prefixKey.data[:], k[:])
		curHashes[prefixKey] = hasher.HashLeaf(treeID, k[:], []byte(v))
	}

	for curBitLen > 0 {
		curBitLen--
		curHeight++
		nextHashes := make(map[bitString][]byte)
		for k := range curHashes {
			// If k is an index with bit pattern xyzw, the parent node will have an
			// index with bit pattern xyz (one bit shorter).
			shorterK := truncateToNBits(k, curBitLen)
			if _, ok := nextHashes[shorterK]; ok {
				// We may have both xyz0 and xyz1 in curHashes, in which case
				// we will see the shortened form xyz twice.  This is the second
				// time around.
				continue
			}
			// Look up both possible extensions of the shortened form; at least
			// one (k itself) should be in curHashes.
			shorterKWith0 := extendByABit(shorterK, 0)
			shorterKWith1 := extendByABit(shorterK, 1)
			hL, okL := curHashes[shorterKWith0]
			hR, okR := curHashes[shorterKWith1]
			if !okL && !okR {
				panic(fmt.Sprintf("neither child non-empty for parent of k=%x/%d!", k.data, k.bitLen))
			}
			// Either extension may be absent from curHashes, which implies it has
			// the default hash value for the current level.
			if !okL {
				hL = hasher.HashEmpty(treeID, shorterKWith0.data[:], curHeight-1)
			}
			if !okR {
				hR = hasher.HashEmpty(treeID, shorterKWith1.data[:], curHeight-1)
			}
			nextHashes[shorterK] = hasher.HashChildren(hL, hR)
		}
		curHashes = nextHashes
	}

	// At the end of the process we should be left with a single key {data: []byte{}, bitLen: 0}
	// and the root hash is the corresponding value.
	if len(curHashes) != 1 {
		return nil, fmt.Errorf("reached top of tree but not a single hash (%d)", len(curHashes))
	}
	root := bitString{bitLen: 0}
	rootHash, ok := curHashes[root]
	if !ok {
		return nil, errors.New("missing entry for root hash")
	}
	return rootHash, nil
}

// How many copies of map contents to hold on to.
const copyCount = 10

// VersionedMapContents holds a collection of copies of a Map's
// contents across different revisions.
type VersionedMapContents struct {
	mu sync.RWMutex

	// contents holds copies of the map at different revisions,
	// from later to earlier (so [0] is the most recent).
	contents [copyCount]*MapContents
}

// Empty indicates whether the most recent map contents are empty.
func (p *VersionedMapContents) Empty() bool {
	return p.LastCopy() == nil
}

// PrevCopy returns the specified copy of the map's contents.
func (p *VersionedMapContents) PrevCopy(which int) *MapContents {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.contents[which]
}

// LastCopy returns the most recent copy of the map's contents.
func (p *VersionedMapContents) LastCopy() *MapContents {
	return p.PrevCopy(0)
}

// PickCopy returns a previous copy of the map's contents at random,
// returning nil if there are no local copies.
func (p *VersionedMapContents) PickCopy(prng *rand.Rand) *MapContents {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// Count the number of filled copies.
	i := 0
	for ; i < copyCount; i++ {
		if p.contents[i] == nil {
			break
		}
	}
	if i == 0 {
		// No copied contents yet
		return nil
	}
	choice := prng.Intn(i)
	return p.contents[choice]
}

// UpdateContentsWith stores a new copy of the Map's contents, updating the
// most recent copy with the given leaves.  Returns the updated contents.
func (p *VersionedMapContents) UpdateContentsWith(rev uint64, leaves []*trillian.MapLeaf) (*MapContents, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Sanity check on rev being +ve and monotone increasing.
	if rev < 1 {
		return nil, ErrInvariant{fmt.Sprintf("got rev %d, want >=1 when trying to update hammer state with contents", rev)}
	}
	if p.contents[0] != nil && int64(rev) <= p.contents[0].Rev {
		return nil, ErrInvariant{fmt.Sprintf("got rev %d, want >%d when trying to update hammer state with new contents", rev, p.contents[0].Rev)}
	}

	// Shuffle earlier contents along.
	for i := copyCount - 1; i > 0; i-- {
		p.contents[i] = p.contents[i-1]
	}
	p.contents[0] = p.contents[1].UpdatedWith(rev, leaves)

	if glog.V(3) {
		p.dumpLockedContents()
	}
	return p.contents[0], nil
}

// dumpLockedContents shows the local copies of the map's contents; it should be called with p.mu held.
func (p *VersionedMapContents) dumpLockedContents() {
	fmt.Println("Contents\n~~~~~~~~")
	for i, c := range p.contents {
		if c == nil {
			break
		}
		fmt.Printf(" slot #%d\n", i)
		fmt.Printf("   %s\n", c)
	}
	fmt.Println("~~~~~~~~")
}

// ExtraDataForValue builds a deterministic value for the extra data to be associated
// with a given value.
func ExtraDataForValue(value []byte, sz uint) []byte {
	if len(value) == 0 {
		// Always use empty extra data for an empty value, to prevent confusion between
		// a never-been-set key and a set-then-cleared key.
		return nil
	}
	result := make([]byte, sz)
	copy(result, "extra-"+string(value))
	return result
}

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

// Package compact provides compact Merkle tree data structures.
package compact

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/bits"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/hashers"
)

// RootHashMismatchError indicates an unexpected root hash value.
type RootHashMismatchError struct {
	ExpectedHash []byte
	ActualHash   []byte
}

// Error formats the error into a string.
func (r RootHashMismatchError) Error() string {
	return fmt.Sprintf("root hash mismatch: got %v, expected %v", r.ActualHash, r.ExpectedHash)
}

// NodeID identifies a node of a Merkle tree.
type NodeID struct {
	Level uint
	Index uint64
}

// Tree is a compact Merkle tree representation. It uses O(log(N)) nodes to
// represent the current on-disk tree.
//
// TODO(pavelkalinnikov): Remove it, use compact.Range instead.
type Tree struct {
	hasher hashers.LogHasher
	root   []byte
	// the list of "dangling" left-hand nodes, NOTE: index 0 is the leaf, not the root.
	nodes [][]byte
	size  int64
}

func isPerfectTree(x int64) bool {
	return x != 0 && (x&(x-1) == 0)
}

// GetNodesFunc is a function prototype which can look up particular nodes
// within a non-compact Merkle tree. Used by the compact Tree to populate
// itself with correct state when starting up with a non-empty tree.
type GetNodesFunc func(ids []NodeID) ([][]byte, error)

// NewTreeWithState creates a new compact Tree for the passed in |size|.
//
// This can fail if the nodes required to recreate the tree state cannot be
// fetched or the calculated root hash after population does not match the
// value we expect.
//
// |f| will be called a number of times with the coordinates of internal Merkle
// tree nodes whose hash values are required to initialize the internal state
// of the compact Tree. |expectedRoot| is the known-good tree root of the tree
// at |size|, and is used to verify the initial state of the compact Tree.
func NewTreeWithState(hasher hashers.LogHasher, size int64, f GetNodesFunc, expectedRoot []byte) (*Tree, error) {
	sizeBits := bits.Len64(uint64(size))
	r := Tree{
		hasher: hasher,
		nodes:  make([][]byte, sizeBits),
		root:   hasher.EmptyRoot(),
		size:   size,
	}

	if isPerfectTree(size) {
		glog.V(1).Info("Is perfect tree.")
		r.root = append(make([]byte, 0, len(expectedRoot)), expectedRoot...)
		r.nodes[sizeBits-1] = r.root
	} else {
		ids := make([]NodeID, 0, bits.OnesCount64(uint64(size)))
		for sz := uint64(size); sz != 0; sz &= sz - 1 { // Iterate 1-bits of size.
			level := uint(bits.TrailingZeros64(sz))
			index := (sz - 1) >> level
			ids = append(ids, NodeID{Level: level, Index: index})
		}
		hashes, err := f(ids)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch nodes: %v", err)
		}
		if got, want := len(hashes), len(ids); got != want {
			return nil, fmt.Errorf("got %d hashes, needed %d", got, want)
		}

		for level, idx := 0, 0; level < sizeBits; level++ {
			if size&(1<<uint(level)) != 0 {
				r.nodes[level] = hashes[idx]
				idx++
			}
		}
		r.recalculateRoot(func(depth int, index int64, hash []byte) error {
			return nil
		})
	}

	if !bytes.Equal(r.root, expectedRoot) {
		glog.Warningf("Corrupt state, expected root %s, got %s", hex.EncodeToString(expectedRoot[:]), hex.EncodeToString(r.root[:]))
		return nil, RootHashMismatchError{ActualHash: r.root, ExpectedHash: expectedRoot}
	}
	glog.V(1).Infof("Resuming at size %d, with root: %s", r.size, base64.StdEncoding.EncodeToString(r.root[:]))
	return &r, nil
}

// NewTree creates a new compact Tree with size zero. This always succeeds.
func NewTree(hasher hashers.LogHasher) *Tree {
	r := Tree{
		hasher: hasher,
		root:   hasher.EmptyRoot(),
		nodes:  make([][]byte, 0),
		size:   0,
	}
	return &r
}

// CurrentRoot returns the current root hash.
func (t *Tree) CurrentRoot() []byte {
	return t.root
}

type setNodeFunc func(depth int, index int64, hash []byte) error

func (t *Tree) recalculateRoot(f setNodeFunc) error {
	if t.size == 0 {
		return nil
	}

	index := t.size

	var newRoot []byte
	first := true
	mask := int64(1)
	numBits := bits.Len64(uint64(t.size))
	for bit := 0; bit < numBits; bit++ {
		index >>= 1
		if t.size&mask != 0 {
			if first {
				newRoot = t.nodes[bit]
				first = false
			} else {
				newRoot = t.hasher.HashChildren(t.nodes[bit], newRoot)
				if err := f(bit+1, index, newRoot); err != nil {
					return err
				}
			}
		}
		mask <<= 1
	}
	t.root = newRoot
	return nil
}

// AddLeaf calculates the leafhash of |data| and appends it to the tree.
//
// |f| will be called multiple times with the full Merkle tree coordinates of
// nodes whose hash should be updated.
func (t *Tree) AddLeaf(data []byte, f setNodeFunc) (int64, []byte, error) {
	h, err := t.hasher.HashLeaf(data)
	if err != nil {
		return 0, nil, err
	}
	seq, err := t.AddLeafHash(h, f)
	if err != nil {
		return 0, nil, err
	}
	return seq, h, err
}

// AddLeafHash adds the specified |leafHash| to the tree.
//
// |f| is will be called multiple times with the full Merkle tree coordinates
// of nodes whose hash should be updated.
func (t *Tree) AddLeafHash(leafHash []byte, f setNodeFunc) (int64, error) {
	defer func() {
		t.size++
		// TODO(al): do this lazily
		// TODO(pavelkalinnikov): Handle recalculateRoot errors.
		t.recalculateRoot(f)
	}()

	assignedSeq := t.size
	index := assignedSeq

	if err := f(0, index, leafHash); err != nil {
		return 0, err
	}

	if t.size == 0 {
		// new tree
		t.nodes = append(t.nodes, leafHash)
		return assignedSeq, nil
	}

	// Initialize our running hash value to the leaf hash
	hash := leafHash
	bit := 0
	// Iterate over the bits in our tree size
	for mask := t.size; mask > 0; mask >>= 1 {
		index >>= 1
		if mask&1 == 0 {
			// Just store the running hash here; we're done.
			t.nodes[bit] = hash
			// Don't re-write the leaf hash node (we've done it above already)
			if bit > 0 {
				// Store the leaf hash node
				if err := f(bit, index, hash); err != nil {
					return 0, err
				}
			}
			return assignedSeq, nil
		}
		// The bit is set so we have a node at that position in the nodes list so hash it with our running hash:
		hash = t.hasher.HashChildren(t.nodes[bit], hash)
		// Store the resulting parent hash.
		if err := f(bit+1, index, hash); err != nil {
			return 0, err
		}
		// Now, clear this position in the nodes list as the hash it formerly contained will be propagated upwards.
		t.nodes[bit] = nil
		// Figure out if we're done:
		if bit+1 >= len(t.nodes) {
			// If we're extending the node list then add a new entry with our
			// running hash, and we're done.
			t.nodes = append(t.nodes, hash)
			return assignedSeq, nil
		} else if mask&0x02 == 0 {
			// If the node above us is unused at this tree size, then store our
			// running hash there, and we're done.
			t.nodes[bit+1] = hash
			return assignedSeq, nil
		}
		// Otherwise, go around again.
		bit++
	}
	// We should never get here, because that'd mean we had a running hash which
	// we've not stored somewhere.
	return 0, fmt.Errorf("AddLeaf failed running hash not cleared: h: %v seq: %d", leafHash, assignedSeq)
}

// Size returns the current size of the tree, that is, the number of leaves
// ever added to the tree.
func (t *Tree) Size() int64 {
	return t.size
}

// Hashes returns a copy of the set of node hashes that comprise the compact
// representation of the tree, in the old format.
//
// TODO(pavelkalinnikov): Get rid of this format.
func (t *Tree) Hashes() [][]byte {
	if isPerfectTree(t.size) {
		return nil
	}
	n := make([][]byte, len(t.nodes))
	copy(n, t.nodes)
	return n
}

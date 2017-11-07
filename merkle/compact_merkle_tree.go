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

package merkle

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	log "github.com/golang/glog"
	"github.com/google/trillian/merkle/hashers"
)

// RootHashMismatchError indicates a unexpected root hash value.
type RootHashMismatchError struct {
	ExpectedHash []byte
	ActualHash   []byte
}

func (r RootHashMismatchError) Error() string {
	return fmt.Sprintf("root hash mismatch got: %v expected: %v", r.ActualHash, r.ExpectedHash)
}

// CompactMerkleTree is a compact Merkle tree representation.
// Uses log(n) nodes to represent the current on-disk tree.
type CompactMerkleTree struct {
	hasher hashers.LogHasher
	root   []byte
	// the list of "dangling" left-hand nodes, NOTE: index 0 is the leaf, not the root.
	nodes [][]byte
	size  int64
}

func isPerfectTree(x int64) bool {
	return x != 0 && (x&(x-1) == 0)
}

func bitLen(x int64) int {
	r := 0
	for x > 0 {
		r++
		x >>= 1
	}
	return r
}

// GetNodeFunc is a function prototype which can look up particular nodes within a non-compact Merkle tree.
// Used by the CompactMerkleTree to populate itself with correct state when starting up with a non-empty tree.
type GetNodeFunc func(depth int, index int64) ([]byte, error)

// NewCompactMerkleTreeWithState creates a new CompactMerkleTree for the passed in |size|.
// This can fail if the nodes required to recreate the tree state cannot be fetched or the calculated
// root hash after population does not match the value we expect.
// |f| will be called a number of times with the co-ordinates of internal MerkleTree nodes whose hash values are
// required to initialize the internal state of the CompactMerkleTree.  |expectedRoot| is the known-good tree root
// of the tree at |size|, and is used to verify the correct initial state of the CompactMerkleTree after initialisation.
func NewCompactMerkleTreeWithState(hasher hashers.LogHasher, size int64, f GetNodeFunc, expectedRoot []byte) (*CompactMerkleTree, error) {
	sizeBits := bitLen(size)

	r := CompactMerkleTree{
		hasher: hasher,
		nodes:  make([][]byte, sizeBits),
		root:   hasher.EmptyRoot(),
		size:   size,
	}

	if isPerfectTree(size) {
		log.V(1).Info("Is perfect tree.")
		r.root = append(make([]byte, 0, len(expectedRoot)), expectedRoot...)
		r.nodes[sizeBits-1] = r.root
	} else {
		// Pull in the nodes we need to repopulate our compact tree and verify the root
		for depth := 0; depth < sizeBits; depth++ {
			if size&1 == 1 {
				index := size - 1
				log.V(1).Infof("fetching d: %d i: %d, leaving size %d", depth, index, size)
				h, err := f(depth, index)
				if err != nil {
					log.Warningf("Failed to fetch node depth %d index %d: %s", depth, index, err)
					return nil, err
				}
				r.nodes[depth] = h
			}
			size >>= 1
		}
		r.recalculateRoot(func(depth int, index int64, hash []byte) error {
			return nil
		})
	}
	if !bytes.Equal(r.root, expectedRoot) {
		log.Warningf("Corrupt state, expected root %s, got %s", hex.EncodeToString(expectedRoot[:]), hex.EncodeToString(r.root[:]))
		return nil, RootHashMismatchError{ActualHash: r.root, ExpectedHash: expectedRoot}
	}
	log.V(1).Infof("Resuming at size %d, with root: %s", r.size, base64.StdEncoding.EncodeToString(r.root[:]))
	return &r, nil
}

// NewCompactMerkleTree creates a new CompactMerkleTree with size zero. This always succeeds.
func NewCompactMerkleTree(hasher hashers.LogHasher) *CompactMerkleTree {
	r := CompactMerkleTree{
		hasher: hasher,
		root:   hasher.EmptyRoot(),
		nodes:  make([][]byte, 0),
		size:   0,
	}
	return &r
}

// CurrentRoot returns the current root hash.
func (c CompactMerkleTree) CurrentRoot() []byte {
	return c.root
}

// DumpNodes logs the internal state of the CompactMerkleTree, and is used for debugging.
func (c CompactMerkleTree) DumpNodes() {
	log.Infof("Tree Nodes @ %d", c.size)
	mask := int64(1)
	numBits := bitLen(c.size)
	for bit := 0; bit < numBits; bit++ {
		if c.size&mask != 0 {
			log.Infof("%d:  %s", bit, base64.StdEncoding.EncodeToString(c.nodes[bit][:]))
		} else {
			log.Infof("%d:  -", bit)
		}
		mask <<= 1
	}
}

type setNodeFunc func(depth int, index int64, hash []byte) error

func (c *CompactMerkleTree) recalculateRoot(f setNodeFunc) error {
	if c.size == 0 {
		return nil
	}

	index := c.size

	var newRoot []byte
	first := true
	mask := int64(1)
	numBits := bitLen(c.size)
	for bit := 0; bit < numBits; bit++ {
		index >>= 1
		if c.size&mask != 0 {
			if first {
				newRoot = c.nodes[bit]
				first = false
			} else {
				newRoot = c.hasher.HashChildren(c.nodes[bit], newRoot)
				if err := f(bit+1, index, newRoot); err != nil {
					return err
				}
			}
		}
		mask <<= 1
	}
	c.root = newRoot
	return nil
}

// AddLeaf calculates the leafhash of |data| and appends it to the tree.
// |f| is a callback which will be called multiple times with the full MerkleTree coordinates of nodes whose hash should be updated.
func (c *CompactMerkleTree) AddLeaf(data []byte, f setNodeFunc) (int64, []byte, error) {
	h, err := c.hasher.HashLeaf(data)
	if err != nil {
		return 0, nil, err
	}
	seq, err := c.AddLeafHash(h, f)
	if err != nil {
		return 0, nil, err
	}
	return seq, h, err
}

// AddLeafHash adds the specified |leafHash| to the tree.
// |f| is a callback which will be called multiple times with the full MerkleTree coordinates of nodes whose hash should be updated.
func (c *CompactMerkleTree) AddLeafHash(leafHash []byte, f setNodeFunc) (int64, error) {
	defer func() {
		c.size++
		// TODO(al): do this lazily
		c.recalculateRoot(f)
	}()

	assignedSeq := c.size
	index := assignedSeq

	if err := f(0, index, leafHash); err != nil {
		return 0, err
	}

	if c.size == 0 {
		// new tree
		c.nodes = append(c.nodes, leafHash)
		return assignedSeq, nil
	}

	// Initialize our running hash value to the leaf hash
	hash := leafHash
	bit := 0
	// Iterate over the bits in our tree size
	for t := c.size; t > 0; t >>= 1 {
		index >>= 1
		if t&1 == 0 {
			// Just store the running hash here; we're done.
			c.nodes[bit] = hash
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
		hash = c.hasher.HashChildren(c.nodes[bit], hash)
		// Store the resulting parent hash.
		if err := f(bit+1, index, hash); err != nil {
			return 0, err
		}
		// Now, clear this position in the nodes list as the hash it formerly contained will be propagated upwards.
		c.nodes[bit] = nil
		// Figure out if we're done:
		if bit+1 >= len(c.nodes) {
			// If we're extending the node list then add a new entry with our
			// running hash, and we're done.
			c.nodes = append(c.nodes, hash)
			return assignedSeq, nil
		} else if t&0x02 == 0 {
			// If the node above us is unused at this tree size, then store our
			// running hash there, and we're done.
			c.nodes[bit+1] = hash
			return assignedSeq, nil
		}
		// Otherwise, go around again.
		bit++
	}
	// We should never get here, because that'd mean we had a running hash which
	// we've not stored somewhere.
	return 0, fmt.Errorf("AddLeaf failed running hash not cleared: h: %v seq: %d", leafHash, assignedSeq)
}

// Size returns the current size of the tree, that is, the number of leaves ever added to the tree.
func (c CompactMerkleTree) Size() int64 {
	return c.size
}

// Hashes returns a copy of the set of node hashes that comprise the compact representation of the tree.
func (c CompactMerkleTree) Hashes() [][]byte {
	if isPerfectTree(c.size) {
		return nil
	}
	n := make([][]byte, len(c.nodes))
	copy(n, c.nodes)
	return n
}

// Depth returns the number of levels in the tree.
func (c CompactMerkleTree) Depth() int {
	if c.size == 0 {
		return 0
	}
	return bitLen(c.size - 1)
}

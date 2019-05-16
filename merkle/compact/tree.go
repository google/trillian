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

// Tree is a compact Merkle tree representation. It uses O(log(size)) nodes to
// represent the current on-disk tree.
//
// TODO(pavelkalinnikov): Remove it, use compact.Range instead.
type Tree struct {
	hasher hashers.LogHasher
	// The list of "dangling" left-hand nodes, where entry [0] is the leaf.
	// So: nodes[0] is the hash of a subtree of size 1 = 1<<0, if included.
	//     nodes[1] is the hash of a subtree of size 2 = 1<<1, if included.
	//     nodes[2] is the hash of a subtree of size 4 = 1<<2, if included.
	//     ....
	// Nodes are included if the tree size includes that power of two.
	// For example, a tree of size 21 is built from subtrees of sizes
	// 16 + 4 + 1, so nodes[1] == nodes[3] == nil.
	//
	// For a tree whose size is a perfect power of two, only the last
	// entry in nodes will be set (which is also the root hash).
	nodes [][]byte
	size  int64
}

func isPerfectTree(size int64) bool {
	return size != 0 && (size&(size-1) == 0)
}

// GetNodesFunc is a function prototype which can look up particular nodes
// within a non-compact Merkle tree. Used by the compact Tree to populate
// itself with correct state when starting up with a non-empty tree.
type GetNodesFunc func(ids []NodeID) ([][]byte, error)

// NewTreeWithState creates a new compact Tree for the passed in size.
//
// This can fail if the nodes required to recreate the tree state cannot be
// fetched or the calculated root hash after population does not match the
// expected value.
//
// getNodesFn will be called with the coordinates of internal Merkle tree nodes
// whose hash values are required to initialize the internal state of the
// compact Tree. The expectedRoot is the known-good tree root of the tree at
// the specified size, and is used to verify the initial state.
func NewTreeWithState(hasher hashers.LogHasher, size int64, getNodesFn GetNodesFunc, expectedRoot []byte) (*Tree, error) {
	sizeBits := bits.Len64(uint64(size))
	r := Tree{
		hasher: hasher,
		nodes:  make([][]byte, sizeBits),
		size:   size,
	}

	ids := make([]NodeID, 0, bits.OnesCount64(uint64(size)))
	// Iterate over perfect subtrees along the right border of the tree. Those
	// correspond to the bits of the tree size that are set to one.
	for sz := uint64(size); sz != 0; sz &= sz - 1 {
		level := uint(bits.TrailingZeros64(sz))
		index := (sz - 1) >> level
		ids = append(ids, NewNodeID(level, index))
	}
	hashes, err := getNodesFn(ids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nodes: %v", err)
	}
	if got, want := len(hashes), len(ids); got != want {
		return nil, fmt.Errorf("got %d hashes, needed %d", got, want)
	}
	for i, id := range ids {
		r.nodes[id.Level] = hashes[i]
	}

	// TODO(pavelkalinnikov): This check should be done externally.
	root, err := r.CurrentRoot()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(root, expectedRoot) {
		glog.Warningf("Corrupt state, expected root %s, got %s", hex.EncodeToString(expectedRoot[:]), hex.EncodeToString(root))
		return nil, fmt.Errorf("root hash mismatch: got %v, expected %v", root, expectedRoot)
	}

	glog.V(1).Infof("Resuming at size %d, with root: %s", r.size, base64.StdEncoding.EncodeToString(root))
	return &r, nil
}

// NewTree creates a new compact Tree with size zero.
func NewTree(hasher hashers.LogHasher) *Tree {
	return &Tree{
		hasher: hasher,
		nodes:  make([][]byte, 0),
		size:   0,
	}
}

// CurrentRoot returns the current root hash.
func (t *Tree) CurrentRoot() ([]byte, error) {
	return t.CalculateRoot(nil)
}

// String describes the internal state of the compact Tree.
func (t *Tree) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Tree Nodes @ %d\n", t.size))
	mask := int64(1)
	numBits := bits.Len64(uint64(t.size))
	for bit := 0; bit < numBits; bit++ {
		if t.size&mask != 0 {
			buf.WriteString(fmt.Sprintf("%d:  %s\n", bit, base64.StdEncoding.EncodeToString(t.nodes[bit][:])))
		} else {
			buf.WriteString(fmt.Sprintf("%d:  -\n", bit))
		}
		mask <<= 1
	}
	return buf.String()
}

// CalculateRoot computes the current root hash. If visit function is not nil,
// then CalculateRoot calls it for all imperfect subtree roots on the right
// border of the tree (also called "ephemeral" nodes), ordered from lowest to
// highest levels.
func (t *Tree) CalculateRoot(visit VisitFn) ([]byte, error) {
	if t.size == 0 {
		return t.hasher.EmptyRoot(), nil
	}

	index := uint64(t.size)

	var hash []byte
	first := true
	mask := int64(1)
	numBits := uint(bits.Len64(uint64(t.size)))
	for bit := uint(0); bit < numBits; bit++ {
		index >>= 1
		if t.size&mask != 0 {
			if first {
				hash = t.nodes[bit]
				first = false
			} else {
				hash = t.hasher.HashChildren(t.nodes[bit], hash)
				if visit != nil {
					visit(NewNodeID(bit+1, index), hash)
				}
			}
		}
		mask <<= 1
	}
	return hash, nil
}

// AppendLeaf calculates the Merkle leaf hash of the given leaf data and
// appends it to the tree. Returns the Merkle hash of the new leaf. See
// AppendLeafHash for details on how the visit function is used.
func (t *Tree) AppendLeaf(data []byte, visit VisitFn) ([]byte, error) {
	h := t.hasher.HashLeaf(data)
	if err := t.AppendLeafHash(h, visit); err != nil {
		return nil, err
	}
	return h, nil
}

// AppendLeafHash appends a leaf node with the specified hash to the tree.
//
// If visit function is not nil, it will be called for each updated Merkle tree
// node which became a root of a perfect subtree after adding the new leaf.
// Note that this includes the leaf node itself. Ephemeral nodes (roots of
// imperfect subtrees) on the right border of the tree are not visited for
// efficiency reasons, but one can do so by calling the CalculateRoot method -
// typically, after a series of AppendLeafHash calls.
//
// If returns an error then the Tree is no longer usable.
func (t *Tree) AppendLeafHash(leafHash []byte, visit VisitFn) error {
	defer func() { t.size++ }()

	assignedSeq := t.size
	index := uint64(assignedSeq)

	if visit != nil {
		visit(NewNodeID(0, index), leafHash)
	}

	if t.size == 0 {
		// new tree
		t.nodes = append(t.nodes, leafHash)
		return nil
	}

	// Initialize our running hash value to the leaf hash.
	hash := leafHash
	bit := uint(0)
	// Iterate over the bits in our existing tree size.
	for mask := t.size; mask > 0; mask >>= 1 {
		index >>= 1
		if mask&1 == 0 {
			// Just store the running hash here; we're done.
			t.nodes[bit] = hash
			// Don't re-write the leaf hash node (we've done it above already)
			if bit > 0 && visit != nil {
				// Store the (non-leaf) hash node
				visit(NewNodeID(bit, index), hash)
			}
			return nil
		}
		// The bit is set so we have a node at that position in the nodes list so hash it with our running hash:
		hash = t.hasher.HashChildren(t.nodes[bit], hash)
		// Store the resulting parent hash.
		if visit != nil {
			visit(NewNodeID(bit+1, index), hash)
		}
		// Now, clear this position in the nodes list as the hash it formerly contained will be propagated upwards.
		t.nodes[bit] = nil
		// Figure out if we're done:
		if bit+1 >= uint(len(t.nodes)) {
			// If we're extending the node list then add a new entry with our
			// running hash, and we're done.
			t.nodes = append(t.nodes, hash)
			return nil
		} else if mask&0x02 == 0 {
			// If the node above us is unused at this tree size, then store our
			// running hash there, and we're done.
			t.nodes[bit+1] = hash
			return nil
		}
		// Otherwise, go around again.
		bit++
	}
	// We should never get here, because that'd mean we had a running hash which
	// we've not stored somewhere.
	return fmt.Errorf("AddLeaf failed running hash not cleared: h: %v seq: %d", leafHash, assignedSeq)
}

// Size returns the current size of the tree.
func (t *Tree) Size() int64 {
	return t.size
}

// Hashes returns a copy of the set of node hashes that comprise the compact
// representation of the tree. A tree whose size is a power of two has no
// internal node hashes (just the root hash), so returns nil.
//
// TODO(pavelkalinnikov): Get rid of this format, it is only used internally.
func (t *Tree) Hashes() [][]byte {
	if isPerfectTree(t.size) {
		return nil
	}
	n := make([][]byte, len(t.nodes))
	copy(n, t.nodes)
	return n
}

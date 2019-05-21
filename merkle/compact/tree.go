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
	rng    *Range
}

// NewTreeWithState creates a new compact Tree for the passed in size.
//
// This can fail if the number of hashes does not correspond to the tree size,
// or the calculated root hash does not match the passed in expected value.
//
// hashes is the list of node hashes that comprise the compact tree. The list
// of the corresponding node IDs that the caller can use to retrieve these
// hashes can be obtained using the TreeNodes function.
//
// The expectedRoot is the known-good tree root of the tree at the specified
// size, and is used to verify the initial state.
func NewTreeWithState(hasher hashers.LogHasher, size int64, hashes [][]byte, expectedRoot []byte) (*Tree, error) {
	fact := RangeFactory{Hash: hasher.HashChildren}
	rng, err := fact.NewRange(0, uint64(size), hashes)
	if err != nil {
		return nil, err
	}

	// TODO(pavelkalinnikov): This check should be done externally.
	t := Tree{hasher: hasher, rng: rng}
	root, err := t.CurrentRoot()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(root, expectedRoot) {
		glog.Warningf("Corrupt state, expected root %s, got %s", hex.EncodeToString(expectedRoot[:]), hex.EncodeToString(root))
		return nil, fmt.Errorf("root hash mismatch: got %v, expected %v", root, expectedRoot)
	}

	glog.V(1).Infof("Loaded tree at size %d, root: %s", rng.End(), base64.StdEncoding.EncodeToString(root))
	return &t, nil
}

// NewTree creates a new compact Tree with size zero.
func NewTree(hasher hashers.LogHasher) *Tree {
	fact := RangeFactory{Hash: hasher.HashChildren}
	rng := fact.NewEmptyRange(0)
	return &Tree{hasher: hasher, rng: rng}
}

// CurrentRoot returns the current root hash.
func (t *Tree) CurrentRoot() ([]byte, error) {
	return t.CalculateRoot(nil)
}

// String describes the internal state of the compact Tree.
//
// TODO(pavelkalinnikov): Remove this method, or move it to Range type.
func (t *Tree) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Tree Nodes @ %d\n", t.rng.End()))
	mask := uint64(1)
	numBits := bits.Len64(t.rng.End())
	for bit, idx := 0, 0; bit < numBits; bit++ {
		if t.rng.End()&mask != 0 {
			buf.WriteString(fmt.Sprintf("%d:  %s\n", bit, base64.StdEncoding.EncodeToString(t.rng.hashes[idx])))
			idx++
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
	if t.rng.End() == 0 {
		return t.hasher.EmptyRoot(), nil
	}
	return t.rng.GetRootHash(visit)
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
	// Report the leaf hash, as the compact.Range doesn't.
	if visit != nil {
		visit(NewNodeID(0, t.rng.End()), leafHash)
	}
	// Report all new perfect internal nodes.
	return t.rng.Append(leafHash, visit)
}

// Size returns the current size of the tree.
func (t *Tree) Size() int64 {
	return int64(t.rng.End())
}

// hashes returns the set of node hashes that comprise the compact
// representation of the tree.
func (t *Tree) hashes() [][]byte {
	return t.rng.Hashes()
}

// TreeNodes returns the list of node IDs that comprise a compact tree, in the
// same order they are used in compact.Tree and compact.Range, i.e. ordered
// from upper to lower levels.
func TreeNodes(size uint64) []NodeID {
	ids := make([]NodeID, 0, bits.OnesCount64(size))
	// Iterate over perfect subtrees along the right border of the tree. Those
	// correspond to the bits of the tree size that are set to one.
	for sz := size; sz != 0; sz &= sz - 1 {
		level := uint(bits.TrailingZeros64(sz))
		index := (sz - 1) >> level
		ids = append(ids, NewNodeID(level, index))
	}
	// Note: Right border nodes of compact.Range are ordered from root to leaves.
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

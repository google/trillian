// Copyright 2022 Google LLC. All Rights Reserved.
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

// Package inmemory provides an in-memory Merkle tree implementation.
package inmemory

import (
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/proof"
)

// Tree implements an append-only Merkle tree. For testing.
//
// This type is immutable, but, if Append* methods are not used carefully, the
// hashes can be corrupted. The semantics of memory reuse and reallocation is
// similar to that of the append built-in for Go slices: overlapping Trees can
// share memory.
//
// It is recommended to reuse old versions of Tree only for read operations, or
// making sure that the newer Trees are no longer used before writing to an
// older Tree. One scenario when rolling back to an older Tree can be useful is
// implementing transaction semantics, rollbacks, and snapshots.
type Tree struct {
	hasher merkle.LogHasher
	size   uint64
	hashes [][][]byte // Node hashes, indexed by node (level, index).
}

// New returns a new empty Merkle tree.
func New(hasher merkle.LogHasher) Tree {
	return Tree{hasher: hasher}
}

// AppendData returns a new Tree which is the result of appending the hashes of
// the given entries and the dependent Merkle tree nodes to the current tree.
//
// See Append method comment for details on safety.
func (t Tree) AppendData(entries ...[]byte) Tree {
	for _, entry := range entries {
		t.appendImpl(t.hasher.HashLeaf(entry))
	}
	return t
}

// Append returns a new Tree which is the result of appending the given leaf
// hashes and the dependent Merkle tree nodes to the current tree.
//
// The new Tree likely shares data with the old Tree, but in such a way that
// both objects are valid. It is safe to reuse / roll back to the older Tree
// objects, but Append should be called on them with caution because it may
// corrupt hashes in the newer Tree objects.
func (t Tree) Append(hashes ...[]byte) Tree {
	for _, hash := range hashes {
		t.appendImpl(hash)
	}
	return t
}

func (t *Tree) appendImpl(hash []byte) {
	level, width := 0, t.size
	for ; width&1 == 1; width, level = width/2, level+1 {
		t.hashes[level] = append(t.hashes[level][:width], hash)
		hash = t.hasher.HashChildren(t.hashes[level][width-1], hash)
	}
	if level > len(t.hashes) {
		panic("gap in tree appends")
	} else if level == len(t.hashes) {
		t.hashes = append(t.hashes, nil)
	}

	t.hashes[level] = append(t.hashes[level][:width], hash)
	t.size++
}

// Size returns the current number of leaves in the tree.
func (t Tree) Size() uint64 {
	return t.size
}

// LeafHash returns the leaf hash at the given index.
// Requires 0 <= index < Size(), otherwise panics.
func (t Tree) LeafHash(index uint64) []byte {
	return t.hashes[0][index]
}

// Hash returns the current root hash of the tree.
func (t Tree) Hash() []byte {
	return t.HashAt(t.size)
}

// HashAt returns the root hash at the given size.
// Requires 0 <= size <= Size(), otherwise panics.
func (t Tree) HashAt(size uint64) []byte {
	if size == 0 {
		return t.hasher.EmptyRoot()
	}
	hashes := t.getNodes(compact.RangeNodes(0, size))

	hash := hashes[len(hashes)-1]
	for i := len(hashes) - 2; i >= 0; i-- {
		hash = t.hasher.HashChildren(hashes[i], hash)
	}
	return hash
}

// InclusionProof returns the inclusion proof for the given leaf index in the
// tree of the given size. Requires 0 <= index < size <= Size(), otherwise may
// panic.
func (t Tree) InclusionProof(index, size uint64) ([][]byte, error) {
	nodes, err := proof.Inclusion(index, size)
	if err != nil {
		return nil, err
	}
	return nodes.Rehash(t.getNodes(nodes.IDs), t.hasher.HashChildren)
}

// ConsistencyProof returns the consistency proof between the two given tree
// sizes. Requires 0 <= size1 <= size2 <= Size(), otherwise may panic.
func (t Tree) ConsistencyProof(size1, size2 uint64) ([][]byte, error) {
	nodes, err := proof.Consistency(size1, size2)
	if err != nil {
		return nil, err
	}
	return nodes.Rehash(t.getNodes(nodes.IDs), t.hasher.HashChildren)
}

func (t Tree) getNodes(ids []compact.NodeID) [][]byte {
	hashes := make([][]byte, len(ids))
	for i, id := range ids {
		hashes[i] = t.hashes[id.Level][id.Index]
	}
	return hashes
}

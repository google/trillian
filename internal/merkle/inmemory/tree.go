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
type Tree struct {
	hasher merkle.LogHasher
	size   uint64
	hashes [][][]byte // Node hashes, indexed by node (level, index).
}

// New returns a new empty Merkle tree.
func New(hasher merkle.LogHasher) *Tree {
	return &Tree{hasher: hasher}
}

// AppendData adds the leaf hashes of the given entries to the end of the tree.
func (t *Tree) AppendData(entries ...[]byte) {
	for _, data := range entries {
		t.appendImpl(t.hasher.HashLeaf(data))
	}
}

// Append adds the given leaf hashes to the end of the tree.
func (t *Tree) Append(hashes ...[]byte) {
	for _, hash := range hashes {
		t.appendImpl(hash)
	}
}

func (t *Tree) appendImpl(hash []byte) {
	level := 0
	for ; (t.size>>level)&1 == 1; level++ {
		row := append(t.hashes[level], hash)
		hash = t.hasher.HashChildren(row[len(row)-2], hash)
		t.hashes[level] = row
	}
	if level > len(t.hashes) {
		panic("gap in tree appends")
	} else if level == len(t.hashes) {
		t.hashes = append(t.hashes, nil)
	}

	t.hashes[level] = append(t.hashes[level], hash)
	t.size++
}

// Size returns the current number of leaves in the tree.
func (t *Tree) Size() uint64 {
	return t.size
}

// LeafHash returns the leaf hash at the given index.
// Requires 0 <= index < Size(), otherwise panics.
func (t *Tree) LeafHash(index uint64) []byte {
	return t.hashes[0][index]
}

// Hash returns the current root hash of the tree.
func (t *Tree) Hash() []byte {
	return t.HashAt(t.size)
}

// HashAt returns the root hash at the given size.
// Requires 0 <= size <= Size(), otherwise panics.
func (t *Tree) HashAt(size uint64) []byte {
	if size == 0 {
		return t.hasher.EmptyRoot()
	}
	hashes := t.getNodes(compact.RangeNodes(0, size, nil))

	hash := hashes[len(hashes)-1]
	for i := len(hashes) - 2; i >= 0; i-- {
		hash = t.hasher.HashChildren(hashes[i], hash)
	}
	return hash
}

// InclusionProof returns the inclusion proof for the given leaf index in the
// tree of the given size. Requires 0 <= index < size <= Size(), otherwise may
// panic.
func (t *Tree) InclusionProof(index, size uint64) ([][]byte, error) {
	nodes, err := proof.Inclusion(index, size)
	if err != nil {
		return nil, err
	}
	return nodes.Rehash(t.getNodes(nodes.IDs), t.hasher.HashChildren)
}

// ConsistencyProof returns the consistency proof between the two given tree
// sizes. Requires 0 <= size1 <= size2 <= Size(), otherwise may panic.
func (t *Tree) ConsistencyProof(size1, size2 uint64) ([][]byte, error) {
	nodes, err := proof.Consistency(size1, size2)
	if err != nil {
		return nil, err
	}
	return nodes.Rehash(t.getNodes(nodes.IDs), t.hasher.HashChildren)
}

func (t *Tree) getNodes(ids []compact.NodeID) [][]byte {
	hashes := make([][]byte, len(ids))
	for i, id := range ids {
		hashes[i] = t.hashes[id.Level][id.Index]
	}
	return hashes
}

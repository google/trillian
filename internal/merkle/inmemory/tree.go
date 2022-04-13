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
	"fmt"

	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/proof"
)

// Tree implements an append-only Merkle tree. For testing.
type Tree struct {
	h    merkle.LogHasher
	size uint64
	n    [][][]byte // Node hashes, indexed by node (level, index).
}

// New returns a new empty Merkle tree.
func New(hasher merkle.LogHasher) *Tree {
	return &Tree{h: hasher}
}

// AppendData adds the leaf hash of the given entry to the end of the tree.
func (t *Tree) AppendData(data []byte) {
	t.Append(t.h.HashLeaf(data))
}

// Append adds the given leaf hash to the end of the tree.
func (t *Tree) Append(hash []byte) {
	level := 0
	for ; (t.size>>level)&1 == 1; level++ {
		row := append(t.n[level], hash)
		hash = t.h.HashChildren(row[len(row)-2], hash)
		t.n[level] = row
	}
	if level > len(t.n) {
		panic("gap in tree appends")
	} else if level == len(t.n) {
		t.n = append(t.n, nil)
	}

	t.n[level] = append(t.n[level], hash)
	t.size++
}

// Size returns the current number of leaves in the tree.
func (t *Tree) Size() uint64 {
	return t.size
}

// LeafHash returns the leaf hash at the specified index.
func (t *Tree) LeafHash(index uint64) []byte {
	return t.n[0][index]
}

// Hash returns the current root hash of the tree.
func (t *Tree) Hash() ([]byte, error) {
	return t.HashAt(t.size)
}

// HashAt returns the root hash at the given size. The size must not exceed the
// current tree size.
func (t *Tree) HashAt(size uint64) ([]byte, error) {
	if size == 0 {
		return t.h.EmptyRoot(), nil
	}
	hashes, err := t.getNodes(compact.RangeNodes(0, size))
	if err != nil {
		return nil, err
	}

	hash := hashes[len(hashes)-1]
	for i := len(hashes) - 2; i >= 0; i-- {
		hash = t.h.HashChildren(hashes[i], hash)
	}
	return hash, nil
}

// InclusionProof returns the inclusion proof for the given leaf index in the
// tree of the given size. The size must not exceed the current tree size.
func (t *Tree) InclusionProof(index, size uint64) ([][]byte, error) {
	nodes, err := proof.Inclusion(index, size)
	if err != nil {
		return nil, err
	}
	return t.getProof(nodes)
}

// ConsistencyProof returns the consistency proof between the two given tree
// sizes. Requires 0 <= size1 <= size2 <= Size().
func (t *Tree) ConsistencyProof(size1, size2 uint64) ([][]byte, error) {
	nodes, err := proof.Consistency(size1, size2)
	if err != nil {
		return nil, err
	}
	return t.getProof(nodes)
}

func (t *Tree) getProof(nodes proof.Nodes) ([][]byte, error) {
	hashes, err := t.getNodes(nodes.IDs)
	if err != nil {
		return nil, err
	}
	return nodes.Rehash(hashes, t.h.HashChildren)
}

func (t *Tree) getNodes(ids []compact.NodeID) ([][]byte, error) {
	hashes := make([][]byte, len(ids))
	for i, id := range ids {
		if id.Level >= uint(len(t.n)) || id.Index >= uint64(len(t.n[id.Level])) {
			return nil, fmt.Errorf("node %+v not found", id)
		}
		hashes[i] = t.n[id.Level][id.Index]
	}
	return hashes, nil
}

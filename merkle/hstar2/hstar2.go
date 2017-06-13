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

// Package hstar2 implements the HStar2 Sparse Merkle Tree hashing algorithm.
// Warning: hstar2 does not provide full n/2 bit length security in the multi-tree setting.
package hstar2

import (
	"crypto"
	"fmt"
)

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// HStar2 computes empty leaf nodes as e0 = H(nil).
// Empty branches within the tree are plain interior nodes e1 = H(e0, e0) etc.
type HStar2 struct {
	crypto.Hash
	nullHashes [][]byte
}

// New creates a new HStar2 MapHasher on the passed in hash function.
func New(h crypto.Hash) *HStar2 {
	m := &HStar2{Hash: h}
	m.nullHashes = m.createNullHashes()
	return m
}

// HashEmpty returns the hash of an empty branch at a given depth.
// A depth of 0 indictes the root of an empty tree.
func (m *HStar2) HashEmpty(depth int) []byte {
	if depth < 0 || depth >= len(m.nullHashes) {
		panic(fmt.Sprintf("HashEmpty(%v) out of bounds", depth))
	}
	return m.nullHashes[depth]
}

// HashLeaf computes the hash of a leaf that exists.
func (m *HStar2) HashLeaf(leaf []byte) []byte {
	h := m.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren computes interior nodes.
func (m *HStar2) HashChildren(l, r []byte) []byte {
	h := m.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

// createNullHashes returns a list of empty hashes starting with the hash of an empty
// tree, all the way down to the hash of an empty leaf.
func (m *HStar2) createNullHashes() [][]byte {
	// Leaves are stored at depth Size()*8.  Root is stored at 0.
	// There are Size()*8 edges, and Size()*8 + 1 nodes in the tree.
	nodes := m.Size()*8 + 1
	r := make([][]byte, nodes, nodes)
	r[nodes-1] = m.HashLeaf(nil)
	for i := nodes - 2; i >= 0; i-- {
		r[i] = m.HashChildren(r[i+1], r[i+1])
	}
	return r
}

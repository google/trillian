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

// Package rfc6962 provides hashing functionality according to RFC6962.
package rfc6962

import (
	"crypto"
	"fmt"
)

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// DefaultHasher is a SHA256 based TreeHasher.
var DefaultHasher = New(crypto.SHA256)

// TreeHasher implements the RFC6962 tree hashing algorithm.
// Empty branches within the tree are plain interior nodes e1 = H(e0, e0) etc.
type TreeHasher struct {
	crypto.Hash
	nullHashes [][]byte
}

// New creates a new TreeHasher on the passed in hash function.
func New(h crypto.Hash) *TreeHasher {
	m := &TreeHasher{Hash: h}
	m.nullHashes = m.createNullHashes()
	return m
}

// String returns a string representation for debugging.
func (t *TreeHasher) String() string {
	return fmt.Sprintf("rfc6962Hash{%v}", t.Hash)
}

// EmptyRoot returns a special case for an empty tree.
func (t *TreeHasher) EmptyRoot() []byte {
	return t.New().Sum(nil)
}

// HashEmpty returns the hash of an empty branch at a given depth.
// A depth of 0 indictes the hash of an empty leaf.
func (t *TreeHasher) HashEmpty(depth int) []byte {
	if depth < 0 || depth >= len(t.nullHashes) {
		panic(fmt.Sprintf("HashEmpty(%v) out of bounds", depth))
	}
	return t.nullHashes[depth]
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t *TreeHasher) HashLeaf(leaf []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t *TreeHasher) HashChildren(l, r []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

// createNullHashes returns a list of empty hashes starting with the hash of an empty
// tree, all the way down to the hash of an empty leaf.
func (t *TreeHasher) createNullHashes() [][]byte {
	// Leaves are stored at depth 0. Root is at Size()*8.
	// There are Size()*8 edges, and Size()*8 + 1 nodes in the tree.
	nodes := t.Size()*8 + 1
	r := make([][]byte, nodes, nodes)
	r[0] = t.HashLeaf(nil)
	for i := 1; i < nodes; i++ {
		r[i] = t.HashChildren(r[i-1], r[i-1])
	}
	return r
}

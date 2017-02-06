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

// Package rfc6962 implements the hasher according to the RFC6962 spec.
package rfc6962

import (
	"crypto"
	_ "crypto/sha256" // Register SHA256.
)

// Hasher implements the hasher according to the RFC6962 spec.
type Hasher struct {
	crypto.Hash
}

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// HashEmpty returns the hash of an empty element for the tree
func (t Hasher) HashEmpty() []byte {
	return t.New().Sum(nil)
	//return t.HashLeaf([]byte{})
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t Hasher) HashLeaf(leaf []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t Hasher) HashChildren(l, r []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

// Size returns the number of bytes in output hashes.
func (t Hasher) Size() int {
	return t.New().Size()
}

// NullHash returns the empty hash at a given depth.
func (t Hasher) NullHash(depth int) []byte {
	h := t.HashLeaf([]byte{})
	//h := t.HashEmpty()
	height := t.Size() * 8
	for i := height - 1; i > depth; i-- {
		h = t.HashChildren(h, h)
	}
	return h
}

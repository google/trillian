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
	"crypto"
	"fmt"
)

// Hasher defines hashing functions for use in tree operations.
type Hasher interface {
	HashLeaf(leaf []byte) []byte
	HashEmpty() []byte
	NullHash(depth int) []byte
	HashChildren(left, right []byte) []byte
	Size() int
}

var hashTypes = map[string]Hasher{
	"RFC6962-SHA256": rfc6962{crypto.SHA256},
}

// Factory returns hashers of given types.
func Factory(t string) (Hasher, error) {
	h, ok := hashTypes[t]
	if !ok {
		return nil, fmt.Errorf("hash type %s not found", t)
	}
	return h, nil
}

//
// RFC6962 Hasher
//

type rfc6962 struct {
	crypto.Hash
}

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// HashEmpty returns the hash of an empty element for the tree
func (t rfc6962) HashEmpty() []byte {
	return t.HashLeaf([]byte{})
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t rfc6962) HashLeaf(leaf []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t rfc6962) HashChildren(l, r []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

func (t rfc6962) Size() int {
	return t.New().Size()
}

// NullHash returns the empty hash at a given depth.
func (t rfc6962) NullHash(depth int) []byte {
	h := t.HashEmpty()
	for i := t.Size()*8 - 1; i > depth; i-- {
		h = t.HashChildren(h, h)
	}
	return h
}

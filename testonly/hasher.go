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

package testonly

// This file implements the hashing functions that are part of a Trillian
// personality.

import (
	"crypto"
	"crypto/sha256"
)

// Hasher is the default hasher for tests.
// TODO: Make this a custom algorithm to decouple hashing from coded defaults.
var Hasher = rfc6962{crypto.SHA256}

// HashKey converts a map key into a map index using SHA256.
// This preserves tests that precomputed indexes based on SHA256.
func HashKey(key string) []byte {
	h := sha256.New()
	h.Write([]byte(key))
	return h.Sum(nil)
}

// TransparentHash returns a key that can be visually inspected.
// This supports testing where it was nice to see what the key was.
func TransparentHash(key string) []byte {
	if len(key) > sha256.Size {
		panic("key too long")
	}
	b := make([]byte, sha256.Size)
	copy(b, key)
	return b
}

//
// Test RFC6962 Hasher
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
	h := t.New()
	return h.Sum(nil)
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
	for i := t.Size() * 8; i >= depth; i-- {
		h = t.HashChildren(h, h)
	}
	return h
}

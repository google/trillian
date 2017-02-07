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
)

// TODO(al): investigate whether we need configurable TreeHashers for
// different users. Apparently E2E hashes in tree-level to the internal nodes
// for example, and some users may want different domain separation prefixes
// etc.
//
// BIG SCARY COMMENT:
//
// We don't want this code to have to depend on or constrain implementations for
// specific applications but we haven't decided how we're going to split domain
// specific stuff from the generic yet and we don't want to lose track of the fact
// that this hashing needs to be domain aware to some extent.
//
// END OF BIG SCARY COMMENT

// Domain separation prefixes
// TODO(Martin2112): Move anything CT specific out of here to <handwave> look over there
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// TreeHasher implements the RFC6962 tree hashing algorithm.
type TreeHasher struct {
	crypto.Hash
}

// NewRFC6962TreeHasher creates a new TreeHasher based on the passed in hash function.
// TODO(Martin2112): Move anything CT specific out of here to <handwave> look over there
func NewRFC6962TreeHasher() TreeHasher {
	return TreeHasher{Hash: crypto.SHA256}
}

// HashEmpty returns the hash of an empty element for the tree
func (t TreeHasher) HashEmpty() []byte {
	return t.New().Sum(nil)
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t TreeHasher) HashLeaf(leaf []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t TreeHasher) HashChildren(l, r []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

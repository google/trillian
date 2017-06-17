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
	_ "crypto/sha256" // SHA256 is the default algorithm.
)

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// DefaultHasher is a SHA256 based TreeHasher.
var DefaultHasher = New(crypto.SHA256)

// RFCHasher implements the RFC6962 tree hashing algorithm.
type RFCHasher struct {
	crypto.Hash
	nullHashes [][]byte
}

// New creates a new merkle.TreeHasher on the passed in hash function.
func New(h crypto.Hash) *RFCHasher {
	return &RFCHasher{Hash: h}
}

// EmptyRoot returns a special case for an empty tree.
func (t *RFCHasher) EmptyRoot() []byte {
	return t.New().Sum(nil)
}

// HashEmpty returns the hash of an empty branch at a given depth.
// A depth of 0 indictes the hash of an empty leaf.
func (t *RFCHasher) HashEmpty(depth int) []byte {
	panic("HashEmpty() is not implemented for rfc6962 hasher")
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t *RFCHasher) HashLeaf(leaf []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t *RFCHasher) HashChildren(l, r []byte) []byte {
	h := t.New()
	h.Write([]byte{RFC6962NodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

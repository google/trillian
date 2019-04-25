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
	"hash"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
)

func init() {
	hashers.RegisterLogHasher(trillian.HashStrategy_RFC6962_SHA256, New(crypto.SHA256))
	hashers.RegisterInplaceLogHasher(trillian.HashStrategy_RFC6962_SHA256,
		func() hashers.InplaceLogHasher {
			return NewInplace(crypto.SHA256)
		})
}

// Domain separation prefixes
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// DefaultHasher is a SHA256 based LogHasher.
var DefaultHasher = New(crypto.SHA256)

// Hasher implements the RFC6962 tree hashing algorithm.
type Hasher struct {
	crypto.Hash
}

// InplaceHasher implements the RFC6962 tree hashing algorithm.
type InplaceHasher struct {
	crypto.Hash
	hasher hash.Hash
	buffer []byte
}

// New creates a new hashers.LogHasher on the passed in hash function.
func New(h crypto.Hash) *Hasher {
	return &Hasher{Hash: h}
}

// NewInplace creates a new hashers.InplaceLogHasher on the passed in hash function.
func NewInplace(h crypto.Hash) *InplaceHasher {
	return &InplaceHasher{Hash: h, hasher: h.New()}
}

// EmptyRoot returns a special case for an empty tree.
func (t *Hasher) EmptyRoot() []byte {
	return t.New().Sum(nil)
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t *Hasher) HashLeaf(leaf []byte) ([]byte, error) {
	h := t.New()
	h.Write([]byte{RFC6962LeafHashPrefix})
	h.Write(leaf)
	return h.Sum(nil), nil
}

// HashChildren returns the inner Merkle tree node hash of the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t *Hasher) HashChildren(l, r []byte) []byte {
	h := t.New()
	b := append(append(append(
		make([]byte, 0, 1+len(l)+len(r)),
		RFC6962NodeHashPrefix),
		l...),
		r...)

	h.Write(b)
	return h.Sum(nil)
}

// EmptyRootInto returns a special case for an empty tree.
func (t *InplaceHasher) EmptyRootInto(res []byte) []byte {
	t.hasher.Reset()
	return t.hasher.Sum(res)
}

// HashLeafInto places the Merkle tree leaf hash of the data passed in into a
// supplied slice, which can be reused if appropriate. The data in leaf is
// prefixed by the LeafHashPrefix. Note: This function is not thread safe.
func (t *InplaceHasher) HashLeafInto(leaf, res []byte) ([]byte, error) {
	t.hasher.Reset()
	t.hasher.Write([]byte{RFC6962LeafHashPrefix})
	t.hasher.Write(leaf)
	if res == nil {
		res = make([]byte, 0, t.hasher.Size())
	}
	res = res[:0]
	return t.hasher.Sum(res), nil
}

// HashChildrenInto places the inner Merkle tree node hash of the two child nodes
// l and r into a supplied slice, which can be reused if appropriate. The hashed
// structure is NodeHashPrefix||l||r. Note: This function is not thread safe.
func (t *InplaceHasher) HashChildrenInto(l, r, res []byte) []byte {
	t.buffer = t.buffer[:0]
	t.buffer = append(append(append(
		t.buffer,
		RFC6962NodeHashPrefix),
		l...),
		r...)

	t.hasher.Reset()
	t.hasher.Write(t.buffer)
	res = res[:0]
	return t.hasher.Sum(res)
}

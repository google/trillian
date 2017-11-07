// Copyright 2017 Google Inc. All Rights Reserved.
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

// Package coniks provides hashing for maps.
package coniks

import (
	"crypto"
	"encoding/binary"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
)

func init() {
	hashers.RegisterMapHasher(trillian.HashStrategy_CONIKS_SHA512_256, Default)
}

// Domain separation prefixes
var (
	leafIdentifier  = []byte("L")
	emptyIdentifier = []byte("E")
)

// Default is the standard CONIKS hasher.
var Default = New(crypto.SHA512_256)

// hasher implements the sparse merkle tree hashing algorithm specified in the CONIKS paper.
type hasher struct {
	crypto.Hash
}

// New creates a new hashers.TreeHasher using the passed in hash function.
func New(h crypto.Hash) hashers.MapHasher {
	return &hasher{Hash: h}
}

// EmptyRoot returns the root of an empty tree.
func (m *hasher) EmptyRoot() []byte {
	panic("EmptyRoot() not defined for coniks.Hasher")
}

// HashEmpty returns the hash of an empty branch at a given height.
// A height of 0 indicates the hash of an empty leaf.
// Empty branches within the tree are plain interior nodes e1 = H(e0, e0) etc.
func (m *hasher) HashEmpty(treeID int64, index []byte, height int) []byte {
	depth := m.BitLen() - height

	h := m.New()
	h.Write(emptyIdentifier)
	binary.Write(h, binary.BigEndian, uint64(treeID))
	h.Write(m.maskIndex(index, depth))
	binary.Write(h, binary.BigEndian, uint32(depth))
	r := h.Sum(nil)
	glog.V(5).Infof("HashEmpty(%x, %d): %x", index, depth, r)
	return r
}

// HashLeaf calculate the merkle tree leaf value:
// H(Identifier || treeID || depth || index || dataHash)
func (m *hasher) HashLeaf(treeID int64, index []byte, leaf []byte) ([]byte, error) {
	depth := m.BitLen()
	h := m.New()
	h.Write(leafIdentifier)
	binary.Write(h, binary.BigEndian, uint64(treeID))
	h.Write(m.maskIndex(index, depth))
	binary.Write(h, binary.BigEndian, uint32(depth))
	h.Write(leaf)
	p := h.Sum(nil)
	glog.V(5).Infof("HashLeaf(%x, %d, %s): %x", index, depth, leaf, p)
	return p, nil
}

// HashChildren returns the internal Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is  H(l || r).
func (m *hasher) HashChildren(l, r []byte) []byte {
	h := m.New()
	h.Write(l)
	h.Write(r)
	p := h.Sum(nil)
	glog.V(5).Infof("HashChildren(%x, %x): %x", l, r, p)
	return p
}

// BitLen returns the number of bits in the hash function.
func (m *hasher) BitLen() int {
	return m.Size() * 8
}

// leftmask contains bitmasks indexed such that the left x bits are set. It is
// indexed by byte position from 0-7 0 is special cased to 0xFF since 8 mod 8
// is 0. leftmask is only used to mask the last byte.
var leftmask = [8]byte{0xFF, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFC, 0xFE}

// maskIndex returns index with only the left depth bits set.
// index must be of size m.Size() and 0 <= depth <= m.BitLen().
// e.g.
func (m *hasher) maskIndex(index []byte, depth int) []byte {
	if got, want := len(index), m.Size(); got != want {
		panic(fmt.Sprintf("index len: %d, want %d", got, want))
	}
	if got, want := depth, m.BitLen(); got < 0 || got > want {
		panic(fmt.Sprintf("depth: %d, want <= %d && > 0", got, want))
	}

	// Create an empty index Size() bytes long.
	ret := make([]byte, m.Size())
	if depth > 0 {
		// Copy the first depthBytes.
		depthBytes := (depth + 7) >> 3
		copy(ret, index[:depthBytes])
		// Mask off unwanted bits in the last byte.
		ret[depthBytes-1] = ret[depthBytes-1] & leftmask[depth%8]
	}
	return ret
}

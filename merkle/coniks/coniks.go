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
	"bytes"
	"crypto"
	"encoding/binary"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
)

func init() {
	hashers.RegisterMapHasher(trillian.HashStrategy_CONIKS_SHA512_256, Default)
	hashers.RegisterMapHasher(trillian.HashStrategy_CONIKS_SHA256, New(crypto.SHA256))
}

// Domain separation prefixes
var (
	leafIdentifier  = []byte("L")
	emptyIdentifier = []byte("E")
	// Default is the standard CONIKS hasher.
	Default = New(crypto.SHA512_256)
	// Some zeroes, to avoid allocating temporary slices.
	zeroes = make([]byte, 32)
)

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

	buf := bytes.NewBuffer(make([]byte, 0, 32))
	h := m.New()
	buf.Write(emptyIdentifier)
	binary.Write(buf, binary.BigEndian, uint64(treeID))
	m.writeMaskedIndex(buf, index, depth)
	binary.Write(buf, binary.BigEndian, uint32(depth))
	h.Write(buf.Bytes())
	r := h.Sum(nil)
	if glog.V(5) {
		glog.Infof("HashEmpty(%x, %d): %x", index, depth, r)
	}
	return r
}

// HashLeaf calculate the merkle tree leaf value:
// H(Identifier || treeID || depth || index || dataHash)
func (m *hasher) HashLeaf(treeID int64, index []byte, leaf []byte) []byte {
	depth := m.BitLen()
	buf := bytes.NewBuffer(make([]byte, 0, 32+len(leaf)))
	h := m.New()
	buf.Write(leafIdentifier)
	binary.Write(buf, binary.BigEndian, uint64(treeID))
	m.writeMaskedIndex(buf, index, depth)
	binary.Write(buf, binary.BigEndian, uint32(depth))
	buf.Write(leaf)
	h.Write(buf.Bytes())
	p := h.Sum(nil)
	if glog.V(5) {
		glog.Infof("HashLeaf(%x, %d, %s): %x", index, depth, leaf, p)
	}
	return p
}

// HashChildren returns the internal Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is  H(l || r).
func (m *hasher) HashChildren(l, r []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 32+len(l)+len(r)))
	h := m.New()
	buf.Write(l)
	buf.Write(r)
	h.Write(buf.Bytes())
	p := h.Sum(nil)
	if glog.V(5) {
		glog.Infof("HashChildren(%x, %x): %x", l, r, p)
	}
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

// writeMaskedIndex writes the left depth bits of index directly to a Buffer (which never
// returns an error on writes). This is then padded with zero bits to the Size()
// of the index values in use by this hashes. This avoids the need to allocate
// space for and copy a value that will then be discarded immediately.
func (m *hasher) writeMaskedIndex(b *bytes.Buffer, index []byte, depth int) {
	if got, want := len(index), m.Size(); got != want {
		panic(fmt.Sprintf("index len: %d, want %d", got, want))
	}
	if got, want := depth, m.BitLen(); got < 0 || got > want {
		panic(fmt.Sprintf("depth: %d, want <= %d && >= 0", got, want))
	}

	prevLen := b.Len()
	if depth > 0 {
		// Write the first depthBytes, if there are any complete bytes.
		depthBytes := depth >> 3
		if depthBytes > 0 {
			b.Write(index[:depthBytes])
		}
		// Mask off unwanted bits in the last byte, if there is an incomplete one.
		if depth%8 != 0 {
			b.WriteByte(index[depthBytes] & leftmask[depth%8])
		}
	}
	// Pad to the correct length with zeros. Allow for future hashers that
	// might be > 256 bits.
	needZeros := prevLen + len(index) - b.Len()
	for needZeros > 0 {
		chunkSize := needZeros
		if chunkSize > 32 {
			chunkSize = 32
		}
		b.Write(zeroes[:chunkSize])
		needZeros -= chunkSize
	}
}

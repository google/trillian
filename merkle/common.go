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

// Package merkle provides Merkle tree manipulation functions.
package merkle

// parent returns the index of the parent node in the parent level of the tree.
func parent(leafIndex int64) int64 {
	return leafIndex >> 1
}

// isRightChild returns true if the node is a right child.
func isRightChild(leafIndex int64) bool {
	return leafIndex&1 == 1
}

// bit returns the i'th bit of index from the right.
// eg. bit(0x80000000, 31) -> 1
func bit(index []byte, i int) uint {
	IndexBits := len(index) * 8
	bIndex := (IndexBits - i - 1) / 8
	return uint((index[bIndex] >> uint(i%8)) & 0x01)
}

// flipBit returns index with the i'th bit from the right flipped.
func flipBit(index []byte, i int) []byte {
	r := make([]byte, len(index))
	copy(r, index)
	IndexBits := len(index) * 8
	bIndex := (IndexBits - i - 1) / 8
	r[bIndex] ^= 1 << uint(i%8)
	return r
}

// Neighbor returns index with only the left i bits set and the i'th bit flipped.
func Neighbor(index []byte, i int) []byte {
	r := flipBit(index, i)
	return MaskIndex(r, len(index)*8-i)
}

// leftmask contains bitmasks indexed such that the left x bits are set. It is
// indexed by byte position from 0-7 0 is special cased to 0xFF since 8 mod 8
// is 0. leftmask is only used to mask the last byte.
var leftmask = [8]byte{0xFF, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFC, 0xFE}

// MaskIndex returns index with only the left depth bits set.
func MaskIndex(index []byte, depth int) []byte {
	r := make([]byte, len(index))
	if depth > 0 {
		// Copy the first depthBytes.
		depthBytes := (depth + 7) >> 3
		copy(r, index[:depthBytes])
		// Mask off unwanted bits in the last byte.
		r[depthBytes-1] = r[depthBytes-1] & leftmask[depth%8]
	}
	return r
}

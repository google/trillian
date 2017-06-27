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

package coniks

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"testing"
)

// h2b converts a hex string into a bytes string
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

func TestVectors(t *testing.T) {
	for _, tc := range []struct {
		treeID int64
		index  []byte
		depth  int
		leaf   []byte
		want   []byte
	}{
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 128, []byte(""), h2b("5f4bf72f8175e8db7b58c96354d870b60fb98ce7e4fdde7d601a4d3e5b5d1f20")},
		{1, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 128, []byte(""), h2b("a5f5d0c1e86a15c1ab9c8b88f7e8b7ef17b246350c141c6f21ab81e51d5a6ef2")},
		{0, h2b("1111111111111111111111111111111111111111111111111111111111111111"), 128, []byte(""), h2b("f7ab5ae11bdea50c293a59c0399f5704fd3401ab4144b3ce6230a6866efe2304")},
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 127, []byte(""), h2b("8a8170ff167d7dcdf1b580c89b2f6a6cc3a085c957d1d637d6314e38b83732a0")},
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 128, []byte("foo"), h2b("0d394ddaca7acbf2ad6f9bede5f652be966e3c9e94eaccc472c9b2ca139d06ec")},
		// Test vector from Key Transparency
		{0, h2b("1111111111111111111111111111111111111111111111111111111111111111"), 128, []byte("leaf"), h2b("d77b4bb8e8fdd941976d285a8a0cd8db27b6f7e889e51134e1428224306b6f52")},
	} {
		height := Default.BitLen() - tc.depth
		if got, want := Default.HashLeaf(tc.treeID, tc.index, height, tc.leaf), tc.want; !bytes.Equal(got, want) {
			t.Errorf("HashLeaf(%v, %s, %v, %s): %x, want %x", tc.treeID, tc.index, tc.depth, tc.leaf, got, want)
		}
	}
}

func TestMaskIndex(t *testing.T) {
	h := &hasher{crypto.SHA1} // Use a shorter hash for shorter test vectors.
	for _, tc := range []struct {
		index []byte
		depth int
		want  []byte
	}{
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 0, want: h2b("0000000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 1, want: h2b("8000000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 2, want: h2b("C000000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 3, want: h2b("E000000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 4, want: h2b("F000000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 5, want: h2b("F800000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 6, want: h2b("FC00000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 7, want: h2b("FE00000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 8, want: h2b("FF00000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 9, want: h2b("FF80000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 10, want: h2b("FFC0000000000000000000000000000000000000")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 159, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 1, want: h2b("0000000000000000000000000000000000000000")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 17, want: h2b("0001000000000000000000000000000000000000")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 159, want: h2b("000102030405060708090A0B0C0D0E0F10111212")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
	} {
		if got, want := h.maskIndex(tc.index, tc.depth), tc.want; !bytes.Equal(got, want) {
			t.Errorf("maskIndex(%x, %v): %x, want %x", tc.index, tc.depth, got, want)
		}
	}
}

// Copyright 2017 Google LLC. All Rights Reserved.
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

package hasher

import (
	"bytes"
	"crypto"
	_ "crypto/sha512"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/google/trillian/testonly"
)

var h2b = testonly.MustHexDecode

// Test vectors in this file were computed by first running the tests, then
// saving the outputs.  This is the reference implementation. Implementations
// in other languages use these test vectors to ensure interoperability.
// Any changes to these vectors should be carefully coordinated across all
// known verifiers including:
// - github.com/google/end-to-end/src/javascript/crypto/e2e/transparency

func TestBitLen(t *testing.T) {
	if got, want := Default.BitLen(), 256; got != want {
		t.Errorf("BitLen(): %v, want %v", got, want)
	}
}

func TestHashChildren(t *testing.T) {
	for _, tc := range []struct {
		l, r []byte
		want []byte
	}{
		{nil, nil, h2b("c672b8d1ef56ed28ab87c3622c5114069bdd3ad7b8f9737498d0c01ecef0967a")},
		{h2b("00"), h2b("11"), h2b("248423e26b964db547e51deaffefc7e07bea495283dba563d74d69bf06b87497")},
		{h2b("11"), h2b("00"), h2b("b5fbd3a07d7d309062044c5f92bc32bb860900f6e2a0e63b034846e267cb25bb")},
		{h2b("0000000000000000000000000000000000000000000000000000000000000000"), h2b("1111111111111111111111111111111111111111111111111111111111111111"), h2b("c1c6101db394de0d197b6bd90406fa300d28fee7028c7b37406b16edb61cccb4")},
		{h2b("1111111111111111111111111111111111111111111111111111111111111111"), h2b("0000000000000000000000000000000000000000000000000000000000000000"), h2b("4698c0cfa150974ad8d55e9c3f7d20dcdd56e7023931e1f7b5f61fbcf15770c3")},
	} {
		if got, want := Default.HashChildren(tc.l, tc.r), tc.want; !bytes.Equal(got, want) {
			t.Errorf("HashChildren(%v, %x): %x, want %x", tc.l, tc.r, got, want)
		}
	}
}

func TestHashEmpty(t *testing.T) {
	for _, tc := range []struct {
		treeID int64
		index  []byte
		height int
		want   []byte
	}{
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 256, h2b("2b71932d625e7b83ce864f8092ae4eb470670ccff37eaac83f21679bb3b24bbb")},
		{1, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 256, h2b("9a908ed88f429272254a97c6a55f781e15b0cff753fb90ce7591988b398378ea")},
		{0, h2b("1111111111111111111111111111111111111111111111111111111111111111"), 255, h2b("a9804d4c78c33a72903a5dc71a900a00e55136a425b6e4365c2d90f8303eb233")},
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 0, h2b("af8545ff33b365f2a45971abc45167634c17bfc883ff0280f56e542663b02417")},
	} {
		if got, want := Default.HashEmpty(tc.treeID, tc.index, tc.height), tc.want; !bytes.Equal(got, want) {
			t.Errorf("HashEmpty(%v, %x, %v): %x, want %x", tc.treeID, tc.index, tc.height, got, want)
		}
	}
}

func TestHashLeaf(t *testing.T) {
	for _, tc := range []struct {
		treeID int64
		index  []byte
		leaf   []byte
		want   []byte
	}{
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), nil, h2b("b4e04ff32be7f76c9621dd28946c261dd8aea6494bf713c03da75dd9f1ce2fec")},
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), []byte(""), h2b("b4e04ff32be7f76c9621dd28946c261dd8aea6494bf713c03da75dd9f1ce2fec")},
		{1, h2b("0000000000000000000000000000000000000000000000000000000000000000"), []byte(""), h2b("83800c063525c35afdfe60733fb631be976d06835f79e9914dd268ec2f313721")},
		{0, h2b("1111111111111111111111111111111111111111111111111111111111111111"), []byte(""), h2b("4a95b36a21da32aba1a32d05ae3a1ef200f896c82a7c5ad03da52b17fdcbeb37")},
		{0, h2b("0000000000000000000000000000000000000000000000000000000000000000"), []byte("foo"), h2b("d1f7b835e5ed66fc564b5a9e0a7ca028a6e4ec85a6c7d9d96b2ad6d0c1369700")},
		// Test vector from Key Transparency
		{0, h2b("1111111111111111111111111111111111111111111111111111111111111111"), []byte("leaf"), h2b("87f51e6ceb5a46947fedbd1de543482fb72f7459055d853a841566ef8e43c4a2")},
	} {
		leafHash := Default.HashLeaf(tc.treeID, tc.index, tc.leaf)
		if got, want := leafHash, tc.want; !bytes.Equal(got, want) {
			t.Errorf("HashLeaf(%v, %s, %s): %x, want %x", tc.treeID, tc.index, tc.leaf, got, want)
		}
	}
}

func TestWriteMaskedIndex(t *testing.T) {
	h := &Hasher{crypto.SHA1} // Use a shorter hash for shorter test vectors.
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
		buf := new(bytes.Buffer)
		h.writeMaskedIndex(buf, tc.index, tc.depth)
		if got, want := buf.Bytes(), tc.want; !bytes.Equal(got, want) {
			t.Errorf("writeMaskedIndex(%x, %v): %x, want %x", tc.index, tc.depth, got, want)
		}
	}
}

func TestWriteMaskedIndex512Bits(t *testing.T) {
	h := &Hasher{crypto.SHA512} // Use a hasher with > 32 byte length.
	for _, tc := range []struct {
		index []byte
		depth int
		want  []byte
	}{
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 0,
			want:  h2b("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 1,
			want:  h2b("80000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 2,
			want:  h2b("C0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 3,
			want:  h2b("E0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 4,
			want:  h2b("F0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 5,
			want:  h2b("F8000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 6,
			want:  h2b("FC000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 7,
			want:  h2b("FE000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 8,
			want:  h2b("FF000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 9,
			want:  h2b("FF800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 10,
			want:  h2b("FFC00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 159,
			want:  h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 160,
			want:  h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("000102030405060708090A0B0C0D0E0F10111213FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 1,
			want:  h2b("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("000102030405060708090A0B0C0D0E0F10111213FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 159,
			want:  h2b("000102030405060708090A0B0C0D0E0F101112120000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("000102030405060708090A0B0C0D0E0F10111213FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 160,
			want:  h2b("000102030405060708090A0B0C0D0E0F101112130000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("000102030405060708090A0B0C0D0E0F10111213FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 167,
			want:  h2b("000102030405060708090A0B0C0D0E0F10111213FE00000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
		{
			index: h2b("000102030405060708090A0B0C0D0E0F10111213FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"),
			depth: 168,
			want:  h2b("000102030405060708090A0B0C0D0E0F10111213FF00000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
	} {
		buf := new(bytes.Buffer)
		h.writeMaskedIndex(buf, tc.index, tc.depth)
		if got, want := buf.Bytes(), tc.want; !bytes.Equal(got, want) {
			t.Errorf("writeMaskedIndex(%x, %v): %x, want %x", tc.index, tc.depth, got, want)
		}
	}
}

func TestWriteMaskedIndexBits(t *testing.T) {
	allFF := h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	h := &Hasher{crypto.SHA512} // Use a hasher with > 32 byte length.
	ref := new(big.Int)
	// Go through all the bits, set them one at a time in the big.Int and compare
	// the results against writeMaskedIndex with a depth of that many set bits.
	val := make([]byte, len(allFF))
	for b := 1; b < 512; b++ {
		ref.SetBit(ref, 512-b, 1)
		copy(val, allFF)
		buf := new(bytes.Buffer)
		h.writeMaskedIndex(buf, val, b)
		if got, want := buf.Bytes(), ref.Bytes(); !bytes.Equal(got, want) {
			t.Errorf("bit: %d got: %s, want: %s", b, hex.EncodeToString(got), hex.EncodeToString(want))
		}
	}
}

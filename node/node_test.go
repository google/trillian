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

package node

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/google/trillian/storage/storagepb"
)

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		inPath                 []byte
		inPathLenBits          int
		splitBytes, suffixBits int
		outPrefix              []byte
		outSuffixBits          int
		outSuffix              []byte
	}{
		{h2b("1234567f"), 32, 3, 8, h2b("123456"), 8, h2b("7f")},
		{h2b("1234567f"), 31, 3, 8, h2b("123456"), 7, h2b("7e")},
		{h2b("123456ff"), 29, 3, 8, h2b("123456"), 5, h2b("f8")},
		{h2b("123456ff"), 25, 3, 8, h2b("123456"), 1, h2b("80")},
		{h2b("12345678"), 16, 1, 8, h2b("12"), 8, h2b("34")},
		{h2b("12345678"), 9, 1, 8, h2b("12"), 1, h2b("00")},
		{h2b("12345678"), 8, 0, 8, h2b(""), 8, h2b("12")},
		{h2b("12345678"), 7, 0, 8, h2b(""), 7, h2b("12")},
		{h2b("12345678"), 0, 0, 8, h2b(""), 0, h2b("00")},
		{h2b("70"), 2, 0, 8, h2b(""), 2, h2b("40")},
		{h2b("70"), 3, 0, 8, h2b(""), 3, h2b("60")},
		{h2b("70"), 4, 0, 8, h2b(""), 4, h2b("70")},
		{h2b("70"), 5, 0, 8, h2b(""), 5, h2b("70")},
		{h2b("0003"), 16, 1, 8, h2b("00"), 8, h2b("03")},
		{h2b("0003"), 15, 1, 8, h2b("00"), 7, h2b("02")},
		{h2b("0001000000000000"), 16, 1, 8, h2b("00"), 8, h2b("01")},
		{h2b("0100000000000000"), 8, 0, 8, h2b(""), 8, h2b("01")},
		// Map subtree scenarios
		{h2b("0100000000000000"), 16, 0, 16, h2b(""), 16, h2b("0100")},
		{h2b("0100000000000000"), 32, 0, 32, h2b(""), 32, h2b("01000000")},
	} {
		n := New(tc.inPath).MaskLeft(tc.inPathLenBits)
		p, s := n.Split(tc.splitBytes, tc.suffixBits)
		if got, want := p, New(tc.outPrefix); !Equal(got, want) {
			t.Errorf("%x.Split(%v, %v): prefix %v, want %v",
				tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}
		if got, want := s, NewFromRaw(tc.outSuffixBits, tc.outSuffix); !Equal(got, want) {
			t.Errorf("%x.Split(%v, %v): suffix %v, want %v",
				tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
		}
	}
}

func TestNewNodeIDForTreeCoords(t *testing.T) {
	for _, tc := range []struct {
		height int
		index  int64
		want   *Node
	}{
		{0, 0x01, NewFromRaw(64, h2b("0000000000000001"))},
		{1, 0x01, NewFromRaw(63, h2b("0000000000000002"))},
		{63, 0x01, NewFromRaw(1, h2b("8000000000000000"))},
	} {
		if got, want := NewFromTreeCoords(tc.height, tc.index), tc.want; !Equal(got, want) {
			t.Errorf("NewFromTreeCoords(%d, %x): %v, want: %v", tc.height, tc.index, got, want)
		}
	}
}

func TestNewFromSubtree(t *testing.T) {
	for _, tc := range []struct {
		prefix     []byte
		depth      int32
		index      int64
		subDepth   int
		totalDepth int
		wantPath   []byte
		wantDepth  int
	}{
		{prefix: h2b(""), depth: 8, index: 0, subDepth: 8, totalDepth: 64, wantPath: h2b("0000000000000000"), wantDepth: 8},
		{prefix: h2b(""), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("0100000000000000"), wantDepth: 8},
		{prefix: h2b("00"), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("0001000000000000"), wantDepth: 16},
		{prefix: h2b("00"), depth: 16, index: 257, subDepth: 16, totalDepth: 64, wantPath: h2b("0001010000000000"), wantDepth: 24},
		{prefix: h2b("12345678"), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("1234567801000000"), wantDepth: 40},
	} {
		st := &storagepb.SubtreeProto{
			Prefix: tc.prefix,
			Depth:  tc.depth,
		}
		if got, want := NewFromSubtree(st, tc.subDepth, tc.index, tc.totalDepth),
			NewFromRaw(tc.wantDepth, tc.wantPath); !Equal(got, want) {
			t.Errorf("NewFromSubtree(%x, %v, %v, %v, %v) %v, want %v",
				tc.prefix, tc.depth, tc.index, tc.subDepth, tc.totalDepth, got, want)
		}
	}
}

func TestRightBit(t *testing.T) {
	for _, tc := range []struct {
		index []byte
		i     int
		want  uint
	}{
		{index: h2b("00"), i: 0, want: 0},
		{index: h2b("00"), i: 7, want: 0},
		{index: h2b("000b"), i: 0, want: 1},
		{index: h2b("000b"), i: 1, want: 1},
		{index: h2b("000b"), i: 2, want: 0},
		{index: h2b("000b"), i: 3, want: 1},
		{index: h2b("0001"), i: 0, want: 1},
		{index: h2b("8000"), i: 15, want: 1},
		{index: h2b("0000000000000001"), i: 0, want: 1},
		{index: h2b("0000000000010000"), i: 16, want: 1},
		{index: h2b("8000000000000000"), i: 63, want: 1},
	} {
		n := New(tc.index)
		if got, want := n.RightBit(tc.i), tc.want; got != want {
			t.Errorf("bit(%x, %d): %v, want %v", tc.index, tc.i, got, want)
		}
	}
}

func TestFlipBit(t *testing.T) {
	for _, tc := range []struct {
		index []byte
		i     int
		want  []byte
	}{
		{index: h2b("00"), i: 0, want: h2b("01")},
		{index: h2b("00"), i: 7, want: h2b("80")},
		{index: h2b("000b"), i: 0, want: h2b("000a")},
		{index: h2b("000b"), i: 1, want: h2b("0009")},
		{index: h2b("000b"), i: 2, want: h2b("000f")},
		{index: h2b("000b"), i: 3, want: h2b("0003")},
		{index: h2b("0001"), i: 0, want: h2b("0000")},
		{index: h2b("8000"), i: 15, want: h2b("0000")},
		{index: h2b("0000000000000001"), i: 0, want: h2b("0000000000000000")},
		{index: h2b("0000000000010000"), i: 16, want: h2b("0000000000000000")},
		{index: h2b("8000000000000000"), i: 63, want: h2b("0000000000000000")},
	} {
		if got, want := New(tc.index).FlipRightBit(tc.i), New(tc.want); !Equal(got, want) {
			t.Errorf("FlipRightBit(%x, %d): %x, want %x", tc.index, tc.i, got, want)
		}
	}
}

func TestMaskIndex(t *testing.T) {
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
		n := New(tc.index)
		want := &Node{
			path:  tc.want,
			depth: tc.depth,
		}
		if got := n.MaskLeft(tc.depth); !Equal(got, want) {
			t.Errorf("maskIndex(%x, %v): %x, want %x", tc.index, tc.depth, got, want)
		}
	}
}

func TestSiblings(t *testing.T) {
	for _, tc := range []struct {
		input []byte
		want  []*Node
	}{
		{
			input: h2b("abe4"),
			want: []*Node{
				NewFromRaw(16, h2b("abe5")), // "1010 1011 1110 0101",
				NewFromRaw(15, h2b("abe6")), // "1010 1011 1110 011",
				NewFromRaw(14, h2b("abe0")), // "1010 1011 1110 00",
				NewFromRaw(13, h2b("abe8")), // "1010 1011 1110 1",
				NewFromRaw(12, h2b("abf0")), // "1010 1011 1111",
				NewFromRaw(11, h2b("abc0")), // "1010 1011 110",
				NewFromRaw(10, h2b("ab80")), // "1010 1011 10",
				NewFromRaw(9, h2b("ab00")),  // "1010 1011 0",
				NewFromRaw(8, h2b("aa00")),  // "1010 1010",
				NewFromRaw(7, h2b("a800")),  // "1010 100",
				NewFromRaw(6, h2b("ac00")),  // "1010 11",
				NewFromRaw(5, h2b("a000")),  // "1010 0",
				NewFromRaw(4, h2b("b000")),  // "1011",
				NewFromRaw(3, h2b("8000")),  // "100",
				NewFromRaw(2, h2b("c000")),  // "11",
				NewFromRaw(1, h2b("0000")),  // "0"},
			},
		},
	} {
		sibs := New(tc.input).Siblings()
		if got, want := len(sibs), len(tc.want); got != want {
			t.Errorf("Got %d siblings, want %d", got, want)
			continue
		}

		for i, s := range sibs {
			if got, want := s, tc.want[i]; !Equal(got, want) {
				t.Errorf("sibling %d: %v, want %v", i, got, want)
			}
		}
	}
}

func TestNewFromBig(t *testing.T) {
	for _, tc := range []struct {
		depth      int
		index      *big.Int
		totalDepth int
		wantPath   []byte
		wantDepth  int
	}{
		{256, new(big.Int).SetBytes(h2b("00")), 256, h2b("0000000000000000000000000000000000000000000000000000000000000000"), 256},
		{256, new(big.Int).SetBytes(h2b("01")), 256, h2b("0000000000000000000000000000000000000000000000000000000000000001"), 256},
		{8, new(big.Int).SetBytes(h2b("4100000000000000000000000000000000000000000000000000000000000000")), 256,
			h2b("4100000000000000000000000000000000000000000000000000000000000000"), 8},
	} {
		if got, want := NewFromBig(tc.depth, tc.index, tc.totalDepth),
			NewFromRaw(tc.wantDepth, tc.wantPath); !Equal(got, want) {
			t.Errorf("NewNodeIDFromBigInt(%v, %x, %v): %x, want %x",
				tc.depth, tc.index.Bytes(), tc.totalDepth, got, want)
		}
	}
}

// h2b converts a hex string into []byte.
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

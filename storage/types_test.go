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

package storage

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/google/trillian/testonly"
)

var h2b = testonly.MustHexDecode

func TestMaskLeft(t *testing.T) {
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
		nID := NewNodeIDFromHash(tc.index)
		if got, want := nID.MaskLeft(tc.depth).Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("maskIndex(%x, %v): %x, want %x", tc.index, tc.depth, got, want)
		}
	}
}

func TestNewNodeIDFromBigInt(t *testing.T) {
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
		n := NewNodeIDFromBigInt(tc.depth, tc.index, tc.totalDepth)
		if got, want := n.Path, tc.wantPath; !bytes.Equal(got, want) {
			t.Errorf("NewNodeIDFromBigInt(%v, %x, %v): %x, want %x",
				tc.depth, tc.index.Bytes(), tc.totalDepth, got, want)
		}
		if got, want := n.PrefixLenBits, tc.wantDepth; got != want {
			t.Errorf("NewNodeIDFromBigInt(%v, %x, %v): depth %v, want %v",
				tc.depth, tc.index.Bytes(), tc.totalDepth, got, want)
		}
	}
}

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		inPath                 []byte
		inPathLenBits          int
		splitBytes, suffixBits int
		outPrefix              []byte
		outSuffixBits          int
		outSuffix              []byte
		unusedBytes            int
	}{
		{h2b("1234567f"), 32, 3, 8, h2b("123456"), 8, h2b("7f"), 0},
		{h2b("123456ff"), 29, 3, 8, h2b("123456"), 5, h2b("f8"), 0},
		{h2b("123456ff"), 25, 3, 8, h2b("123456"), 1, h2b("80"), 0},
		{h2b("12345678"), 16, 1, 8, h2b("12"), 8, h2b("34"), 2},
		{h2b("12345678"), 9, 1, 8, h2b("12"), 1, h2b("00"), 2},
		{h2b("12345678"), 8, 0, 8, h2b(""), 8, h2b("12"), 3},
		{h2b("12345678"), 7, 0, 8, h2b(""), 7, h2b("12"), 3},
		{h2b("12345678"), 0, 0, 8, h2b(""), 0, h2b("00"), 3},
		{h2b("70"), 2, 0, 8, h2b(""), 2, h2b("40"), 0},
		{h2b("70"), 3, 0, 8, h2b(""), 3, h2b("60"), 0},
		{h2b("70"), 4, 0, 8, h2b(""), 4, h2b("70"), 0},
		{h2b("70"), 5, 0, 8, h2b(""), 5, h2b("70"), 0},
		{h2b("0003"), 16, 1, 8, h2b("00"), 8, h2b("03"), 0},
		{h2b("0003"), 15, 1, 8, h2b("00"), 7, h2b("02"), 0},
		{h2b("0001000000000000"), 16, 1, 8, h2b("00"), 8, h2b("01"), 6},
		{h2b("0100000000000000"), 8, 0, 8, h2b(""), 8, h2b("01"), 7},
		// Map subtree scenarios
		{h2b("0100000000000000"), 16, 0, 16, h2b(""), 16, h2b("0100"), 6},
		{h2b("0100000000000000"), 32, 0, 32, h2b(""), 32, h2b("01000000"), 4},
		{h2b("0000000000000000000000000000000000000000000000000000000000000001"), 256, 10, 176, h2b("00000000000000000000"), 176, h2b("00000000000000000000000000000000000000000001"), 0},
	} {
		n := NewNodeIDFromHash(tc.inPath)
		n.PrefixLenBits = tc.inPathLenBits

		p, s := n.Split(tc.splitBytes, tc.suffixBits)
		if got, want := p, tc.outPrefix; !bytes.Equal(got, want) {
			t.Errorf("%d, %x.Split(%v, %v): prefix %x, want %x",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}
		if got, want := int(s.Bits), tc.outSuffixBits; got != want {
			t.Errorf("%d, %x.Split(%v, %v): suffix.Bits %v, want %d",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}
		if got, want := s.Path, tc.outSuffix; !bytes.Equal(got, want) {
			t.Errorf("%d, %x.Split(%v, %v).Path: %x, want %x",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}

		newNode := NewNodeIDFromPrefixSuffix(p, s, len(tc.inPath)*8)
		want := []byte{}
		want = append(want, tc.outPrefix...)
		want = append(want, tc.outSuffix...)
		want = append(want, make([]byte, tc.unusedBytes)...)
		if got, want := newNode.Path, want; !bytes.Equal(got, want) {
			t.Errorf("NewNodeIDFromPrefix(%x, %v).Path: %x, want %x", p, s, got, want)
		}
		if got, want := newNode.PrefixLenBits, n.PrefixLenBits; got != want {
			t.Errorf("NewNodeIDFromPrefix(%x, %v).PrefixLenBits: %x, want %x", p, s, got, want)
		}
	}
}

func TestNewNodeIDFromPrefix(t *testing.T) {
	for _, tc := range []struct {
		prefix     []byte
		depth      int
		index      int64
		subDepth   int
		totalDepth int
		wantPath   []byte
		wantDepth  int
	}{
		{prefix: h2b(""), depth: 8, index: 0, subDepth: 8, totalDepth: 64, wantPath: h2b("0000000000000000"), wantDepth: 8},
		{prefix: h2b(""), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("0100000000000000"), wantDepth: 8},
		{prefix: h2b("00"), depth: 7, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("0002000000000000"), wantDepth: 15},
		{prefix: h2b("00"), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("0001000000000000"), wantDepth: 16},
		{prefix: h2b("00"), depth: 16, index: 257, subDepth: 16, totalDepth: 64, wantPath: h2b("0001010000000000"), wantDepth: 24},
		{prefix: h2b("12345678"), depth: 8, index: 1, subDepth: 8, totalDepth: 64, wantPath: h2b("1234567801000000"), wantDepth: 40},
	} {
		n := NewNodeIDFromPrefix(tc.prefix, tc.depth, tc.index, tc.subDepth, tc.totalDepth)
		if got, want := n.Path, tc.wantPath; !bytes.Equal(got, want) {
			t.Errorf("NewNodeIDFromPrefix(%x, %v, %v, %v, %v).Path: %x, want %x",
				tc.prefix, tc.depth, tc.index, tc.subDepth, tc.totalDepth, got, want)
		}
		if got, want := n.PrefixLenBits, tc.wantDepth; got != want {
			t.Errorf("NewNodeIDFromPrefix(%x, %v, %v, %v, %v).Depth: %v, want %v",
				tc.prefix, tc.depth, tc.index, tc.subDepth, tc.totalDepth, got, want)
		}
	}
}

func TestNewNodeIDWithPrefix(t *testing.T) {
	for _, tc := range []struct {
		input    uint64
		inputLen int
		pathLen  int
		maxLen   int
		want     []byte
	}{
		{input: h26("00"), inputLen: 0, pathLen: 0, maxLen: 64, want: h2b("0000000000000000")},
		{input: h26("12345678"), inputLen: 32, pathLen: 32, maxLen: 64, want: h2b("1234567800000000")},
		// top 15 bits of 0x345678 are: 0101 0110 0111 1000
		{input: h26("345678"), inputLen: 15, pathLen: 16, maxLen: 24, want: h2b("acf000")},
	} {
		n := NewNodeIDWithPrefix(tc.input, tc.inputLen, tc.pathLen, tc.maxLen)
		if got, want := n.Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("NewNodeIDWithPrefix(%x, %v, %v, %v).Path: %x, want %x",
				tc.input, tc.inputLen, tc.pathLen, tc.maxLen, got, want)
		}
	}
}

func TestNewNodeIDForTreeCoords(t *testing.T) {
	for _, v := range []struct {
		height     int64
		index      int64
		maxBits    int
		shouldFail bool
		want       string
	}{
		{0, 0x00, 8, false, "00000000"},
		{0, 0x01, 8, false, "00000001"},
		{1, 0x01, 8, false, "0000001"},
		{0, 0x01, 16, false, "0000000000000001"},
		{2, 0x04, 8, false, "000100"},
		{8, 0x01, 16, false, "00000001"},
		{0, 0x80, 8, false, "10000000"},
		{0, 0x01, 64, false, "0000000000000000000000000000000000000000000000000000000000000001"},
		{63, 0x01, 64, false, "1"},
		{63, 0x02, 64, true, "index of 0x02 is too large for given height"},
	} {
		n, err := NewNodeIDForTreeCoords(v.height, v.index, v.maxBits)

		if got, want := err != nil, v.shouldFail; got != want {
			t.Errorf("NewNodeIDForTreeCoords(%d, %x, %d): %v, want failure: %v",
				v.height, v.index, v.maxBits, err, want)
			continue
		}
		if err != nil {
			continue
		}
		if got, want := n.String(), v.want; got != want {
			t.Errorf("NewNodeIDForTreeCoords(%d, %x, %d).String(): '%v', want '%v'",
				v.height, v.index, v.maxBits, got, want)
		}
	}
}

// TestEquivalentTreeCoords uses the log coordinate scheme to ensure that
// the 8 cases of partial byte IDs are tested for equivalence.
func TestEquivalentTreeCoords(t *testing.T) {
	index := int64(18457) // Arbitrary index.
	for l := int64(0); l < 8; l++ {
		n1, err := NewNodeIDForTreeCoords(l, index, 64)
		if err != nil {
			t.Fatal(err)
		}

		for lDelta := int64(0); lDelta < 2; lDelta++ {
			for iDelta := int64(-1); iDelta < 2; iDelta++ {
				n2, err := NewNodeIDForTreeCoords(l+lDelta, index+iDelta, 64)
				if err != nil {
					t.Fatal(err)
				}
				if lDelta == 0 && iDelta == 0 {
					// Nodes with the same coordinates must be equivalent.
					if !n1.Equivalent(n2) || !n2.Equivalent(n1) {
						t.Errorf("NodeIDs for same coords not equivalent at level %d", l)
					}

					continue
				}
				// Different but 'nearby' coordinates must be different NodeIDs.
				if n1.Equivalent(n2) || n2.Equivalent(n1) {
					t.Errorf("NodeIDs unexpectedly equivalent at level %d %d %d", l, lDelta, iDelta)
				}
			}
		}

		index >>= 1
	}
}

func TestEquivalentTrailingBits(t *testing.T) {
	// Set up node IDs with 56 significant bits and length 64.
	n1 := NewNodeIDWithPrefix(h26("12345678"), 56, 56, 64)
	n2 := NewNodeIDWithPrefix(h26("12345678"), 56, 56, 64)
	// Whatever we do to the last byte of the path shouldn't affect their
	// equivalence.
	for b := 0; b < 256; b++ {
		n2.Path[7] = byte(b)
		if !n1.Equivalent(n2) || !n2.Equivalent(n1) {
			t.Errorf("Not equivalent but should be: %v %v", n1, n2)
		}
	}
	// But if we modify the last but one byte they're now different.
	for b := 1; b < 256; b++ {
		n2.Path[6] = n1.Path[6] ^ byte(b)
		if n1.Equivalent(n2) || n2.Equivalent(n1) {
			t.Errorf("Equivalent but should not be: %v %v", n1, n2)
		}
	}
}

// TestNodeIDFromHash tests some specific bit patterns of different lengths
// and that Equivalent is consistent.
func TestNodeEquivalentFromHash(t *testing.T) {
	for _, tc := range []struct {
		name string
		str1 string
		str2 string
		want bool
	}{
		{
			name: "ok",
			str1: "abcdef0987654321",
			str2: "abcdef0987654321",
			want: true,
		},
		{
			name: "ok 8 bit",
			str1: "c7",
			str2: "c7",
			want: true,
		},
		{
			name: "different 8 bit",
			str1: "c7",
			str2: "d7",
		},
		{
			name: "ok 16 bit",
			str1: "1201",
			str2: "1201",
			want: true,
		},
		{
			name: "different 16 bit",
			str1: "1201",
			str2: "0201",
		},
		{
			name: "different but same length",
			str1: "abcdef0987654321",
			str2: "abcdef0987654320",
		},
		{
			name: "different length",
			str1: "abcdef0987654321",
			str2: "abcdef09876543",
		},
		{
			name: "different midway",
			str1: "abcdef0987654321",
			str2: "abcdef0887654321",
		},
	} {
		h1 := mustDecode(tc.str1)
		h2 := mustDecode(tc.str2)

		n1 := NewNodeIDFromHash(h1)
		n2 := NewNodeIDFromHash(h2)

		if n1.Equivalent(n2) != tc.want || n2.Equivalent(n1) != tc.want {
			t.Errorf("TestNodeIDFromHash mismatch: %v", tc)
		}
	}
}

func TestSetBit(t *testing.T) {
	for _, tc := range []struct {
		n    NodeID
		i    int
		b    uint
		want []byte
	}{
		{
			n: NewNodeIDWithPrefix(h26("00"), 0, 64, 64),
			i: 27, b: 1,
			want: h2b("0000000008000000"),
		},
		{
			n: NewNodeIDWithPrefix(h26("00"), 0, 56, 64),
			i: 0, b: 1,
			want: h2b("0000000000000001"),
		},
		{
			n: NewNodeIDWithPrefix(h26("00"), 0, 64, 64),
			i: 27, b: 0,
			want: h2b("0000000000000000"),
		},
	} {
		n := tc.n
		n.SetBit(tc.i, tc.b)
		if got, want := n.Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("%x.SetBit(%v,%v): %v, want %v", tc.n.Path, tc.i, tc.b, got, want)
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
		nID := NewNodeIDFromHash(tc.index)
		if got, want := nID.FlipRightBit(tc.i).Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("flipBit(%x, %d): %x, want %x", tc.index, tc.i, got, want)
		}
	}
}

func TestBit(t *testing.T) {
	for _, tc := range []struct {
		n    NodeID
		want string
	}{
		{
			// Every 3rd bit is 1.
			n:    NewNodeIDWithPrefix(h26("9249"), 16, 16, 16),
			want: "1001001001001001",
		},
		{
			n:    NewNodeIDWithPrefix(h26("0055"), 16, 16, 24),
			want: "000000000101010100000000",
		},
		{
			n:    NewNodeIDWithPrefix(h26("f2"), 8, 0, 24),
			want: "111100100000000000000000",
		},
		{
			n:    NewNodeIDWithPrefix(h26("01"), 1, 8, 24),
			want: "100000000000000000000000",
		},
	} {
		for i, c := range tc.want {
			height := len(tc.want) - 1 - i // Count from right to left.
			if got, want := tc.n.Bit(height), uint(c-'0'); got != want {
				t.Errorf("%v.Bit(%v): %x, want %v", tc.n.String(), height, got, want)
			}
		}
	}
}

func TestString(t *testing.T) {
	for i, tc := range []struct {
		n    NodeID
		want string
	}{
		{
			n:    NewEmptyNodeID(32),
			want: "",
		},
		{
			n:    NewNodeIDWithPrefix(h26("345678"), 24, 32, 32),
			want: "00110100010101100111100000000000",
		},
		{
			n:    NewNodeIDWithPrefix(h26("12345678"), 32, 32, 64),
			want: "00010010001101000101011001111000",
		},
		{
			n:    NewNodeIDWithPrefix(h26("345678"), 15, 16, 24),
			want: fmt.Sprintf("%016b", (0x345678<<1)&0xfffd),
		},
		{
			n:    NewNodeIDWithPrefix(h26("1234"), 15, 16, 16),
			want: "0010010001101000",
		},
		{
			n:    NewNodeIDWithPrefix(h26("f2"), 8, 8, 24),
			want: "11110010",
		},
		{
			n:    NewNodeIDWithPrefix(h26("1234"), 16, 16, 16),
			want: "0001001000110100",
		},
	} {
		if got, want := tc.n.String(), tc.want; got != want {
			t.Errorf("%v: String():  %v,  want '%v'", i, got, want)
		}
	}
}

func TestSiblings(t *testing.T) {
	for _, tc := range []struct {
		input    uint64
		inputLen int
		pathLen  int
		maxLen   int
		want     []string
	}{
		{
			input:    h26("abe4"),
			inputLen: 16,
			pathLen:  16,
			maxLen:   16,
			want: []string{"1010101111100101",
				"101010111110011",
				"10101011111000",
				"1010101111101",
				"101010111111",
				"10101011110",
				"1010101110",
				"101010110",
				"10101010",
				"1010100",
				"101011",
				"10100",
				"1011",
				"100",
				"11",
				"0"},
		},
	} {
		n := NewNodeIDWithPrefix(tc.input, tc.inputLen, tc.pathLen, tc.maxLen)
		sibs := n.Siblings()
		if got, want := len(sibs), len(tc.want); got != want {
			t.Errorf("Got %d siblings, want %d", got, want)
			continue
		}

		for i, s := range sibs {
			if got, want := s.String(), tc.want[i]; got != want {
				t.Errorf("sibling %d: %v, want %v", i, got, want)
			}
		}
	}
}

func TestNodeEquivalent(t *testing.T) {
	l := 16
	na := NewNodeIDWithPrefix(h26("1234"), l, l, l)
	for _, tc := range []struct {
		n1, n2 NodeID
		want   bool
	}{
		{
			// Self is Equal
			n1:   na,
			n2:   na,
			want: true,
		},
		{
			// Equal
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			want: true,
		},
		{
			// Different PrefixLen
			n1:   NewNodeIDWithPrefix(h26("123f"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("123f"), l-1, l, l),
			want: false,
		},
		{
			// Different IDLen
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("1234"), l, l+8, l+8),
			want: false,
		},
		{
			// Different Prefix
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("5432"), l, l, l),
			want: false,
		},
		{
			// Different Prefix 2
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("1235"), l, l, l),
			want: false,
		},
		{
			// Different Prefix 3
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("2234"), l, l, l),
			want: false,
		},
		{
			// Different max len, but that's ok because the prefixes are identical
			n1:   NewNodeIDWithPrefix(h26("1234"), l, l, l),
			n2:   NewNodeIDWithPrefix(h26("1234"), l, l, l*2),
			want: true,
		},
	} {
		if got, want := tc.n1.Equivalent(tc.n2), tc.want; got != want {
			t.Errorf("Equivalent(%v, %v): %v, want %v",
				tc.n1, tc.n2, got, want)
		}
		if got, want := tc.n2.Equivalent(tc.n1), tc.want; got != want {
			t.Errorf("Equivalent(%v, %v): %v, want %v",
				tc.n1, tc.n2, got, want)
		}
	}
}

// It's important to have confidence in the CoordString output as it's used in debugging
func TestCoordString(t *testing.T) {
	// Test some roundtrips for various depths and indices
	for d := 0; d < 37; d++ {
		for i := 0; i < 117; i++ {
			n, err := NewNodeIDForTreeCoords(int64(d), int64(i), 64)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := n.CoordString(), fmt.Sprintf("[d:%d, i:%d]", d, i); got != want {
				t.Errorf("n.CoordString() got: %v, want: %v", got, want)
			}
		}
	}
}

// h26 converts a hex string into an uint64.
func h26(h string) uint64 {
	i, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		panic(err)
	}
	return i
}

func mustDecode(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

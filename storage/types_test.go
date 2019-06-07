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
	"testing"

	"github.com/google/trillian/testonly"
)

var h2b = testonly.MustHexDecode

func TestSetRight(t *testing.T) {
	for _, tc := range []struct {
		val   byte
		index []byte
		depth int
		want  []byte
	}{
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 0, want: h2b("0000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 1, want: h2b("8000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 2, want: h2b("C000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 3, want: h2b("E000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 4, want: h2b("F000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 5, want: h2b("F800000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 6, want: h2b("FC00000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 7, want: h2b("FE00000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 8, want: h2b("FF00000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 9, want: h2b("FF80000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 10, want: h2b("FFC0000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 159, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE")},
		{val: 0x00, index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0x00, index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 1, want: h2b("0000000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 17, want: h2b("0001000000000000000000000000000000000000")},
		{val: 0x00, index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 159, want: h2b("000102030405060708090A0B0C0D0E0F10111212")},
		{val: 0x00, index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 0, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 1, want: h2b("7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 2, want: h2b("3FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 3, want: h2b("1FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 4, want: h2b("0FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 5, want: h2b("07FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 6, want: h2b("03FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 7, want: h2b("01FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 8, want: h2b("00FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 9, want: h2b("007FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 10, want: h2b("003FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 159, want: h2b("0000000000000000000000000000000000000001")},
		{val: 0xFF, index: h2b("0000000000000000000000000000000000000000"), depth: 160, want: h2b("0000000000000000000000000000000000000000")},
		{val: 0xFF, index: h2b("000102030405060708090A0B0C0D0E0F10111212"), depth: 1, want: h2b("7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("000102030405060708090A0B0C0D0E0F10111212"), depth: 17, want: h2b("00017FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{val: 0xFF, index: h2b("000102030405060708090A0B0C0D0E0F10111212"), depth: 159, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
		{val: 0xFF, index: h2b("000102030405060708090A0B0C0D0E0F10111212"), depth: 160, want: h2b("000102030405060708090A0B0C0D0E0F10111212")},
	} {
		nID := NewNodeIDFromHash(tc.index)
		if got, want := nID.SetRight(tc.depth, tc.val).Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("SetRight(%x, %v, 0x%X): %x, want %x", tc.index, tc.depth, tc.val, got, want)
		}
	}
}

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

func TestNewEmptyNodeIDPanic(t *testing.T) {
	for b := 0; b < 64; b++ {
		t.Run(fmt.Sprintf("%dbits", b), func(t *testing.T) {
			// Only multiples of 8 bits should be accepted.
			want := b%8 != 0
			// Unfortunately we have to test for panics.
			defer func() {
				got := recover()
				if (got != nil && !want) || (got == nil && want) {
					t.Errorf("Incorrect panic behaviour got: %v, want: %v", got, want)
				}
			}()
			_ = NewEmptyNodeID(b)
		})
	}
}

// TestNewNodeIDFromPrefixPanic tests the cases where this will panic. To
// succeed these must all be true:
// 1.) totalDepth must be a multiple of 8 and not negative.
// 2.) subDepth must be a multiple of 8 and not negative.
// 3.) depth must be >= 0.
func TestNewNodeIDFromPrefixPanic(t *testing.T) {
	for _, tc := range []struct {
		name       string
		depth      int
		subDepth   int
		totalDepth int
		wantPanic  bool
	}{
		{
			name:       "ok",
			depth:      64,
			totalDepth: 64,
			subDepth:   64,
		},
		{
			name:       "depthnegative",
			depth:      -1,
			totalDepth: 64,
			subDepth:   64,
			wantPanic:  true,
		},
		{
			name:       "subdepthbad",
			depth:      64,
			totalDepth: 64,
			subDepth:   63,
			wantPanic:  true,
		},
		{
			name:       "subdepthnegative",
			depth:      64,
			totalDepth: 64,
			subDepth:   -1,
			wantPanic:  true,
		},
		{
			name:       "totaldepthbad",
			depth:      64,
			totalDepth: 63,
			subDepth:   64,
			wantPanic:  true,
		},
		{
			name:       "totaldepthnegative",
			depth:      64,
			totalDepth: -1,
			subDepth:   64,
			wantPanic:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Unfortunately we have to test for panics.
			defer func() {
				got := recover()
				if (got != nil && !tc.wantPanic) || (got == nil && tc.wantPanic) {
					t.Errorf("Incorrect panic behaviour got: %v, want: %v", got, tc.wantPanic)
				}
			}()
			_ = NewNodeIDFromPrefix([]byte("prefix"), tc.depth, 0, tc.subDepth, tc.totalDepth)
		})
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
		// We should be able to get the same big.Int back. This is used in
		// the HStar2 implementation so should be tested.
		if got, want := n.BigInt(), tc.index; want.Cmp(got) != 0 {
			t.Errorf("NewNodeIDFromBigInt(%v, %x, %v):got:\n%v, want:\n%v",
				tc.depth, tc.index.Bytes(), tc.totalDepth, got, want)
		}
	}
}

func TestNewNodeIDFromBigIntPanic(t *testing.T) {
	for b := 0; b < 64; b++ {
		t.Run(fmt.Sprintf("%dbits", b), func(t *testing.T) {
			// Only multiples of 8 bits should be accepted. This method also
			// fails for 0 bits, unlike NewNodeIDFromPrefix.
			want := (b%8 != 0) || b == 0
			// Unfortunately we have to test for panics.
			defer func() {
				got := recover()
				if (got != nil && !want) || (got == nil && want) {
					t.Errorf("Incorrect panic behaviour (b=%d) got: %v, want: %v", b, got, want)
				}
			}()
			_ = NewNodeIDFromBigInt(12, big.NewInt(234), b)
		})
	}
}

// goldenSplitData is golden test data for operations which split up a given
// NodeID in some way.
var goldenSplitData = []struct {
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
}

func TestPrefix(t *testing.T) {
	for _, tc := range goldenSplitData {
		n := NewNodeIDFromHash(tc.inPath)
		n.PrefixLenBits = tc.inPathLenBits

		p := n.Prefix(tc.splitBytes)
		if got, want := p, tc.outPrefix; !bytes.Equal(got, want) {
			t.Errorf("%d, %x.Prefix(%v): prefix %x, want %x",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, got, want)
			continue
		}
	}
}

func TestPrefixAsKey(t *testing.T) {
	for _, tc := range goldenSplitData {
		n := NewNodeIDFromHash(tc.inPath)
		n.PrefixLenBits = tc.inPathLenBits

		p := n.PrefixAsKey(tc.splitBytes)
		if got, want := p, string(tc.outPrefix); got != want {
			t.Errorf("%d, %x.PrefixAsKey(%v): prefix %x, want %x",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, got, want)
			continue
		}
	}
}

func TestSplit(t *testing.T) {
	for _, tc := range goldenSplitData {
		n := NewNodeIDFromHash(tc.inPath)
		n.PrefixLenBits = tc.inPathLenBits

		p, s := n.Split(tc.splitBytes, tc.suffixBits)
		if got, want := p, tc.outPrefix; !bytes.Equal(got, want) {
			t.Errorf("%d, %x.Split(%v, %v): prefix %x, want %x",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}
		if got, want := int(s.Bits()), tc.outSuffixBits; got != want {
			t.Errorf("%d, %x.Split(%v, %v): suffix.Bits %v, want %d",
				tc.inPathLenBits, tc.inPath, tc.splitBytes, tc.suffixBits, got, want)
			continue
		}
		if got, want := s.Path(), tc.outSuffix; !bytes.Equal(got, want) {
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

func TestSplitPanics(t *testing.T) {
	n, err := NewNodeIDForTreeCoords(8, 4567, 64)
	if err != nil {
		t.Fatal(err)
	}
	// The 2 cases where split should panic are 1.) There are more bits left
	// after the prefix point than have been requested. 2.) Trying to split zero
	// (or -ve) bits off the end of the NodeID.
	for _, tc := range []struct {
		name      string
		pBytes    int
		sBits     int
		wantPanic bool
	}{
		{
			name:   "split8bits",
			pBytes: 6,
			sBits:  8,
		},
		{
			name:   "split16bits",
			pBytes: 5,
			sBits:  16,
		},
		{
			name:      "split15bits",
			pBytes:    5,
			sBits:     15,
			wantPanic: true, // There's 16 bits left, which is more than requested.
		},
		{
			name:      "splitnothing",
			pBytes:    7, // This is 56 bits - same as the PrefixLength so nothing left.
			sBits:     0,
			wantPanic: true,
		},
		{
			name:      "split1bit",
			pBytes:    7, // This is 56 bits - same as the PrefixLength so nothing left.
			sBits:     1, // When there's nothing left any number of bits fails.
			wantPanic: true,
		},
		{
			name:      "splitoutofbounds",
			pBytes:    9, // There aren't 72 bits in the path.
			sBits:     0,
			wantPanic: true,
		},
	} {
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
			// Unfortunately we have to test for panics.
			defer func() {
				got := recover()
				if (got != nil && !tc.wantPanic) || (got == nil && tc.wantPanic) {
					t.Errorf("Incorrect panic behaviour got: %v, want: %v", got, tc.wantPanic)
				}
			}()
			_, _ = n.Split(tc.pBytes, tc.sBits)
		})
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
	n1 := NewNodeIDFromPrefix(h2b("00112233445566"), 0, 0, 8, 64)
	n2 := NewNodeIDFromPrefix(h2b("00112233445566"), 0, 0, 8, 64)
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

func TestNeighbour(t *testing.T) {
	for _, tc := range []struct {
		index []byte
		want  []byte
	}{
		{index: h2b("00"), want: h2b("01")},
		{index: h2b("000b"), want: h2b("000a")},
		{index: h2b("000c"), want: h2b("000d")},
		{index: h2b("000d"), want: h2b("000c")},
		{index: h2b("0009"), want: h2b("0008")},
		{index: h2b("0001"), want: h2b("0000")},
		{index: h2b("8000"), want: h2b("8001")},
		{index: h2b("0000000000000001"), want: h2b("0000000000000000")},
		{index: h2b("0000000000010000"), want: h2b("0000000000010001")},
		{index: h2b("8000000000000000"), want: h2b("8000000000000001")},
	} {
		nID := NewNodeIDFromHash(tc.index)
		if got, want := nID.Neighbor(nID.PrefixLenBits).Path, tc.want; !bytes.Equal(got, want) {
			t.Errorf("flipBit(%x): %x, want %x", tc.index, got, want)
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
			n:    NewNodeIDFromPrefix(h2b("9249"), 16, 0, 8, 16),
			want: "1001001001001001",
		},
		{
			n:    NewNodeIDFromPrefix(h2b("0055"), 16, 0, 16, 24),
			want: "000000000101010100000000",
		},
		{
			n:    NewNodeIDFromPrefix(h2b("f2"), 8, 0, 0, 24),
			want: "111100100000000000000000",
		},
		{
			n:    NewNodeIDFromPrefix(h2b("80"), 1, 0, 8, 24),
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

func TestBitPanics(t *testing.T) {
	n := NewNodeIDFromPrefix(h2b("64"), 0, 0, 8, 8)
	for b := 0; b < 64; b++ {
		t.Run(fmt.Sprintf("%dbits", b), func(t *testing.T) {
			// Testing anything larger than the 7th bit should fail.
			want := b >= 8
			// Unfortunately we have to test for panics.
			defer func() {
				got := recover()
				if (got != nil && !want) || (got == nil && want) {
					t.Errorf("Incorrect panic behaviour got: %v, want: %v", got, want)
				}
			}()
			_ = n.Bit(b)
		})
	}
}

func TestString(t *testing.T) {
	for i, tc := range []struct {
		n       NodeID
		want    string
		wantKey string
	}{
		{
			n:       NewEmptyNodeID(32),
			want:    "",
			wantKey: "0:",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("34567800"), 0, 0, 32, 32),
			want:    "00110100010101100111100000000000",
			wantKey: "32:34567800",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("12345678"), 0, 0, 32, 32),
			want:    "00010010001101000101011001111000",
			wantKey: "32:12345678",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("acf0"), 0, 0, 16, 16),
			want:    fmt.Sprintf("%016b", (0x345678<<1)&0xfffd),
			wantKey: "16:acf0",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("2468"), 0, 0, 16, 16),
			want:    "0010010001101000",
			wantKey: "16:2468",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("f2"), 0, 0, 8, 8),
			want:    "11110010",
			wantKey: "8:f2",
		},
		{
			n:       NewNodeIDFromPrefix(h2b("1234"), 0, 0, 16, 16),
			want:    "0001001000110100",
			wantKey: "16:1234",
		},
		{
			n:       NewNodeIDFromHash([]byte("this is a hash")),
			want:    "0111010001101000011010010111001100100000011010010111001100100000011000010010000001101000011000010111001101101000",
			wantKey: "112:7468697320697320612068617368",
		},
		{
			n:       NewNodeIDFromBigInt(5, big.NewInt(20), 16),
			want:    "00000",
			wantKey: "5:00",
		},
	} {
		if got, want := tc.n.String(), tc.want; got != want {
			t.Errorf("%v: String():  %v,  want '%v'", i, got, want)
		}
		if got, want := tc.n.AsKey(), tc.wantKey; got != want {
			t.Errorf("%v: AsKey():  %v,  want '%v'", i, got, want)
		}
	}
}

func TestSiblings(t *testing.T) {
	for _, tc := range []struct {
		prefix   []byte
		index    int64
		inputLen int
		maxLen   int
		want     []string
	}{
		{
			prefix:   h2b("abe4"),
			index:    0,
			inputLen: 16,
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
		n := NewNodeIDFromPrefix(tc.prefix, 0, tc.index, tc.inputLen, tc.maxLen)
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
	na := NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l)
	for _, tc := range []struct {
		n1, n2 NodeID
		want   bool
	}{
		{
			// Self is Equal.
			n1:   na,
			n2:   na,
			want: true,
		},
		{
			// Equal (different instances).
			n1:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			want: true,
		},
		{
			// Different Index.
			n1:   NewNodeIDFromPrefix(h2b("123f"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("123f"), 1, int64(l), l, l),
			want: false,
		},
		{
			// Different Prefix.
			n1:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("5432"), 0, int64(l), l, l),
			want: false,
		},
		{
			// Different Prefix 2.
			n1:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("1235"), 0, int64(l), l, l),
			want: false,
		},
		{
			// Different Prefix 3.
			n1:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("2234"), 0, int64(l), l, l),
			want: false,
		},
		{
			// Different max len, but that's ok because the prefixes are identical.
			n1:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l),
			n2:   NewNodeIDFromPrefix(h2b("1234"), 0, int64(l), l, l+8),
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

func mustDecode(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

func BenchmarkString(b *testing.B) {
	nID := NewNodeIDFromHash(h2b("000102030405060708090A0B0C0D0E0F10111213"))
	for i := 0; i < b.N; i++ {
		_ = nID.String()
	}
}

func BenchmarkAsKey(b *testing.B) {
	nID := NewNodeIDFromHash(h2b("000102030405060708090A0B0C0D0E0F10111213"))
	for i := 0; i < b.N; i++ {
		_ = nID.AsKey()
	}
}

func BenchmarkSplit(b *testing.B) {
	n := NewNodeIDFromHash(h2b("0000000000000000000000000000000000000000000000000000000000000001"))
	n.PrefixLenBits = 256
	for i := 0; i < b.N; i++ {
		_, _ = n.Split(10, 176)
	}
}

func BenchmarkSuffix(b *testing.B) {
	n := NewNodeIDFromHash(h2b("0000000000000000000000000000000000000000000000000000000000000001"))
	n.PrefixLenBits = 256
	for i := 0; i < b.N; i++ {
		_ = n.Suffix(10, 176)
	}
}

func runBenchmarkNewNodeIDFromBigInt(b *testing.B, f func(int, *big.Int, int) NodeID) {
	b.Helper()
	for i := 0; i < b.N; i++ {
		_ = f(256, new(big.Int).SetBytes(h2b("00")), 256)
	}
}

func BenchmarkNewNodeIDFromBigIntOld(b *testing.B) {
	runBenchmarkNewNodeIDFromBigInt(b, newNodeIDFromBigIntOld)
}

func BenchmarkNewNodeIDFromBigIntNew(b *testing.B) {
	runBenchmarkNewNodeIDFromBigInt(b, NewNodeIDFromBigInt)
}

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
	"strconv"
	"testing"
)

func TestNewNodeIDWithPrefix(t *testing.T) {
	for _, tc := range []struct {
		input    uint64
		inputLen int
		pathLen  int
		maxLen   int
		want     []byte
	}{
		{
			input:    h26("00"),
			inputLen: 0,
			pathLen:  0,
			maxLen:   64,
			want:     h2b("0000000000000000"),
		},
		{
			input:    h26("12345678"),
			inputLen: 32,
			pathLen:  32,
			maxLen:   64,
			want:     h2b("1234567800000000"),
		},
		{
			input:    h26("345678"),
			inputLen: 15,
			pathLen:  16,
			maxLen:   24,
			want:     h2b("acf000"), // top 15 bits of 0x345678 are: 0101 0110 0111 1000
		},
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
		{0, 0x01, 15, false, "000000000000001"},
		{0, 0x01, 16, false, "0000000000000001"},
		{2, 0x04, 8, false, "000100"},
		{8, 0x01, 16, false, "00000001"},
		{8, 0x01, 9, false, "1"},
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

// h2b converts a hex string into []byte.
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

// Copyright 2019 Google LLC. All Rights Reserved.
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
	"fmt"
	"strconv"
	"testing"
)

func TestNewIDWithLast(t *testing.T) {
	const bytes = "\x0A\x0B\x0C\xFA"
	for _, tc := range []struct {
		length uint
		path   string
		last   byte
		bits   uint8
	}{
		{length: 0, last: 0, bits: 0},
		{length: 0, last: 123, bits: 0},
		{length: 1, last: 0, bits: 1},
		{length: 1, last: 123, bits: 1},
		{length: 4, last: 0, bits: 4},
		{length: 5, last: 0xA, bits: 5},
		{length: 5, last: 0xF, bits: 5},
		{length: 7, last: 0xB, bits: 7},
		{length: 8, last: 0xA, bits: 8},
		{length: 9, path: bytes[:1], last: 0, bits: 1},
		{length: 13, path: bytes[:1], last: 0xA, bits: 5},
		{length: 24, path: bytes[:2], last: 0xC, bits: 8},
		{length: 31, path: bytes[:3], last: 0xFB, bits: 7},
		{length: 31, path: bytes[:3], last: 0xFA, bits: 7},
		{length: 32, path: bytes[:3], last: 0xFA, bits: 8},
	} {
		id := NewID(bytes, tc.length)
		t.Run(id.String(), func(t *testing.T) {
			got := NewIDWithLast(tc.path, tc.last, tc.bits)
			if want := id; got != want {
				t.Errorf("NewIDWithLast: %v, want %v", got, want)
			}
		})
	}
}

func TestIDString(t *testing.T) {
	bytes := string([]byte{5, 1, 127})
	for _, tc := range []struct {
		bits uint
		want string
	}{
		{bits: 0, want: "[]"},
		{bits: 1, want: "[0]"},
		{bits: 4, want: "[0000]"},
		{bits: 6, want: "[000001]"},
		{bits: 8, want: "[00000101]"},
		{bits: 16, want: "[00000101 00000001]"},
		{bits: 21, want: "[00000101 00000001 01111]"},
		{bits: 24, want: "[00000101 00000001 01111111]"},
	} {
		t.Run(fmt.Sprintf("bits:%d", tc.bits), func(t *testing.T) {
			id := NewID(bytes, tc.bits)
			if got, want := id.String(), tc.want; got != want {
				t.Errorf("String: got %q, want %q", got, want)
			}
		})
	}
}

func TestIDComparison(t *testing.T) {
	const bytes = "\x0A\x0B\x0C\x0A\x0B\x0C\x01"
	for _, tc := range []struct {
		desc string
		id1  ID
		id2  ID
		want bool
	}{
		{desc: "all-same", id1: NewID(bytes, 56), id2: NewID(bytes, 56), want: true},
		{desc: "same-bytes", id1: NewID(bytes[:3], 24), id2: NewID(bytes[3:6], 24), want: true},
		{desc: "same-bits1", id1: NewID(bytes[:4], 25), id2: NewID(bytes[3:], 25), want: true},
		{desc: "same-bits2", id1: NewID(bytes[:4], 28), id2: NewID(bytes[3:], 28), want: true},
		{desc: "diff-bits", id1: NewID(bytes[:4], 29), id2: NewID(bytes[3:], 29)},
		{desc: "diff-len", id1: NewID(bytes, 56), id2: NewID(bytes, 55)},
		{desc: "diff-bytes", id1: NewID(bytes, 56), id2: NewID(bytes, 48)},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			eq := tc.id1 == tc.id2
			if want := tc.want; eq != want {
				t.Errorf("(id1==id2) is %v, want %v", eq, want)
			}
		})
	}
}

func TestIDPrefix(t *testing.T) {
	const bytes = "\x0A\x0B\x0C"
	for i, tc := range []struct {
		id   ID
		bits uint
		want ID
	}{
		{id: NewID(bytes, 24), bits: 0, want: ID{}},
		{id: NewID(bytes, 24), bits: 1, want: NewID(bytes, 1)},
		{id: NewID(bytes, 24), bits: 2, want: NewID(bytes, 2)},
		{id: NewID(bytes, 24), bits: 5, want: NewID(bytes, 5)},
		{id: NewID(bytes, 24), bits: 8, want: NewID(bytes, 8)},
		{id: NewID(bytes, 24), bits: 15, want: NewID(bytes, 15)},
		{id: NewID(bytes, 24), bits: 24, want: NewID(bytes, 24)},
		{id: NewID(bytes, 21), bits: 15, want: NewID(bytes, 15)},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got, want := tc.id.Prefix(tc.bits), tc.want; got != want {
				t.Errorf("Prefix: %v, want %v", got, want)
			}
		})
	}
}

func TestIDSibling(t *testing.T) {
	const bytes = "\x0A\x0B\x0C"
	for _, tc := range []struct {
		id   ID
		want ID
	}{
		{id: NewID(bytes, 0), want: ID{}},
		{id: NewID(bytes, 1), want: NewID("\xA0", 1)},
		{id: NewID(bytes, 2), want: NewID("\x40", 2)},
		{id: NewID(bytes, 8), want: NewID("\x0B", 8)},
		{id: NewID(bytes, 24), want: NewID("\x0A\x0B\x0D", 24)},
	} {
		t.Run(tc.id.String(), func(t *testing.T) {
			sib := tc.id.Sibling()
			if got, want := sib, tc.want; got != want {
				t.Errorf("Sibling: got %v, want %v", got, want)
			}
			// The sibling's sibling is the original node.
			if got, want := sib.Sibling(), tc.id; got != want {
				t.Errorf("Sibling: got %v, want %v", got, want)
			}
		})
	}
}

func BenchmarkIDSiblings(b *testing.B) {
	siblings := func(id ID) []ID {
		ln := id.BitLen()
		sibs := make([]ID, ln)
		for height := range sibs {
			depth := ln - uint(height)
			sibs[height] = id.Prefix(depth).Sibling()
		}
		return sibs
	}

	const batch = 512
	ids := make([]ID, batch)
	for i := range ids {
		bytes := fmt.Sprintf("0123456789012345678901234567%02x%02x", i&255, (i>>8)&255)
		ids[i] = NewID(bytes, uint(len(bytes))*8)
	}
	for i, n := 0, b.N; i < n; i++ {
		for _, id := range ids {
			_ = siblings(id)
		}
	}
}

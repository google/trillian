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

package merkle

import (
	"bytes"
	"testing"
)

func TestBit(t *testing.T) {
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
		if got, want := bit(tc.index, tc.i), tc.want; got != want {
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
		if got, want := flipBit(tc.index, tc.i), tc.want; !bytes.Equal(got, want) {
			t.Errorf("flipBit(%x, %d): %x, want %x", tc.index, tc.i, got, want)
		}
	}
}

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

package cache

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPrefix(t *testing.T) {
	for _, tc := range []struct {
		index       []byte
		depth, bits int
		want        []byte
	}{
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 0, want: h2b("")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 1, want: h2b("80")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 2, want: h2b("C0")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 3, want: h2b("E0")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 4, want: h2b("F0")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 5, want: h2b("F8")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 6, want: h2b("FC")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 7, want: h2b("FE")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 8, want: h2b("FF")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 9, want: h2b("FF80")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 10, want: h2b("FFC0")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 159, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 160, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 1, want: h2b("00")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 17, want: h2b("000100")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 159, want: h2b("000102030405060708090A0B0C0D0E0F10111212")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 160, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
	} {
		if got, want := prefix(tc.index, tc.depth, tc.bits), tc.want; !bytes.Equal(got, want) {
			t.Errorf("prefix(%x, %v, %v): %x, want %x", tc.index, tc.depth, tc.bits, got, want)
		}
	}
}

func TestSuffix(t *testing.T) {
	for _, tc := range []struct {
		index       []byte
		depth, bits int
		want        []byte
	}{
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 0, want: h2b("")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 1, want: h2b("01")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 2, want: h2b("03")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 3, want: h2b("07")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 4, want: h2b("0F")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 5, want: h2b("1F")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 6, want: h2b("3F")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 7, want: h2b("7F")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 8, want: h2b("FF")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 9, want: h2b("01FF")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 10, want: h2b("03FF")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 159, want: h2b("7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{index: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"), depth: 160, bits: 160, want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 1, want: h2b("01")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 17, want: h2b("011213")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 159, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
		{index: h2b("000102030405060708090A0B0C0D0E0F10111213"), depth: 160, bits: 160, want: h2b("000102030405060708090A0B0C0D0E0F10111213")},
		{index: h2b("1234567f"), depth: 32, bits: 8, want: h2b("7f")},
		// Test depths that don't fall on byte boundaries.
		{index: h2b("0f"), depth: 8, bits: 1, want: h2b("01")},
		{index: h2b("0e"), depth: 7, bits: 1, want: h2b("02")},
		{index: h2b("0f"), depth: 6, bits: 1, want: h2b("04")},
		{index: h2b("0f"), depth: 5, bits: 1, want: h2b("08")},
		{index: h2b("123456ff"), depth: 25, bits: 1, want: h2b("80")}, // 25: 1-111 1111
		{index: h2b("123456ff"), depth: 29, bits: 5, want: h2b("f8")},
	} {
		if got, want := suffix(tc.index, tc.depth, tc.bits), tc.want; !bytes.Equal(got, want) {
			t.Errorf("suffix(%x, %v, %v): %x, want %x", tc.index, tc.depth, tc.bits, got, want)
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

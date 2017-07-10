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
	"fmt"
)

// prefix returns the first prefixBits of index, with the trailing bits masked off.
// eg. prefix(0xFFFF, 16, 9): 0xFF80
func prefix(index []byte, depth, prefixBits int) []byte {
	if got, want := prefixBits, depth; got > want {
		panic(fmt.Sprintf("cache: prefix(_, %v, %v): want %v <= %v", got, want, got, want))
	}

	if prefixBits == 0 {
		return []byte{}
	}

	// Number of bytes to copy. 1 - 8 == 1
	prefixLen := (prefixBits-1)/8 + 1

	r := make([]byte, prefixLen)
	copy(r, index[:prefixLen])

	// Mask off the low bits.
	maskIndex := prefixLen - 1 // Last byte of prefix.
	maskLowBits := uint((prefixBits-1)%8 + 1)
	r[maskIndex] &= ((0x01 << maskLowBits) - 1) << uint(8-maskLowBits)
	return r
}

// suffix returns the bits from prefixBits + 1 through the end of index
// with the unwanted high bits masked off.
// eg. suffix(0xFFFF, 16, 7): 0x7F
func suffix(index []byte, depth, suffixBits int) []byte {
	if got, want := suffixBits, depth; got > want {
		panic(fmt.Sprintf("cache: suffix(_, %v, %v): want %v <= %v", got, want, got, want))
	}

	if suffixBits == 0 {
		return []byte{}
	}

	prefixLen := (depth - suffixBits) / 8 // Bytes to skip: 0-7 = 0
	suffixLen := (suffixBits-1)/8 + 1     // Bytes to copy: 1-8 = 1

	r := make([]byte, suffixLen)
	copy(r, index[prefixLen:])

	// Mask off the unused bits at the end of index.
	maskIndex := suffixLen - 1 // Last byte of suffix.
	maskLowBits := (depth-1)%8 + 1
	r[maskIndex] &= ((0x01 << uint(maskLowBits)) - 1) << uint(8-maskLowBits)

	// Mask off the high bits.
	emptyBits := 8 - maskLowBits
	maskHighBits := uint((emptyBits+suffixBits-1)%8 + 1)
	r[0] &= ((0x01 << maskHighBits) - 1)
	return r
}

// splitIndex returns prefix and suffix
func splitIndex(index []byte, depth, prefixBits int) ([]byte, []byte) {
	suffixBits := depth - prefixBits

	pfx := prefix(index, depth, prefixBits)
	sfx := suffix(index, depth, suffixBits)
	return pfx, sfx
}

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

package cache

import (
	"encoding/base64"
	"errors"
	"fmt"
)

// key is used to look up variable length suffix values in the map.
type key struct {
	// The depth of this entry in bits (must be > 0).
	depth uint8
	// The value of the path at this depth.
	value byte
}

var (
	// emptySuffix is a reusable suffix of zero bits. To avoid special cases
	// there is a single byte path attached to it and there is no way to create
	// a suffix with a nil or empty path.
	emptySuffix = newSuffix(0, []byte{0})
	// fromRaw maps a bit length and single byte path to a suffix.
	fromRaw = make(map[key]*suffix)
	// fromString maps a base64 encoded string representation to a suffix.
	fromString = make(map[string]*suffix)
)

// suffix represents the tail of a NodeID. It is the path within the subtree.
// The portion of the path that extends beyond the subtree is not part of this suffix.
// We keep a cache of the suffix values use by log trees, which will have a
// depth between 1 and 8 bits. These are reused to avoid constant reallocation
// and base64 conversion overhead.
//
// TODO(pavelkalinnikov, v2): This type is specific to SubtreeProto. Move it.
type suffix struct {
	// bits is the number of bits in the node ID suffix.
	bits uint8
	// path is the suffix itself.
	path []byte
	// asString is the string representation of the suffix.
	asString string
}

// newSuffix creates a new suffix. The primary use for them is to get their
// String value to use as a key so we compute that once up front.
//
// TODO(pavelkalinnikov): Mask the last byte of path.
func newSuffix(bits uint8, path []byte) *suffix {
	// Use a shared value for a short suffix if we have one, they're immutable.
	if bits <= 8 {
		if sfx, ok := fromRaw[key{depth: bits, value: path[0]}]; ok {
			return sfx
		}
	}

	r := make([]byte, 1, len(path)+1)
	r[0] = bits
	r = append(r, path...)
	s := base64.StdEncoding.EncodeToString(r)

	return &suffix{bits: bits, path: r[1:], asString: s}
}

// Bits returns the number of significant bits in the suffix path.
func (s suffix) Bits() uint8 {
	return s.bits
}

// Path returns a copy of the suffix path.
func (s suffix) Path() []byte {
	return append(make([]byte, 0, len(s.path)), s.path...)
}

// String returns a string that represents suffix.
// This is a base64 encoding of the following format:
// [ 1 byte for depth || path bytes ]
func (s suffix) String() string {
	return s.asString
}

// parseSuffix converts a suffix string back into a suffix.
func parseSuffix(s string) (*suffix, error) {
	if sfx, ok := fromString[s]; ok {
		// Matches a precalculated value, use that.
		return sfx, nil
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("empty bytes")
	}
	bits, b := b[0], b[1:]
	if got, want := len(b), bytesForBits(int(bits)); got != want {
		return nil, fmt.Errorf("unexpected length %d, need %d", got, want)
	}

	return newSuffix(bits, b), nil
}

// bytesForBits returns the number of bytes required to store numBits bits.
func bytesForBits(numBits int) int {
	return (numBits + 7) >> 3
}

// Precalculate all the one byte suffix values (from depths 1-8) so they can be
// reused either on construction or parsing.
func init() {
	path := make([]byte, 1)
	// There are 8 levels of depth to process.
	for d := 8; d >= 1; d-- {
		// And at each depth there's 2^d valid combinations of bits. E.g at
		// depth 1 there's one valid bit so two possibilities.
		for i := 0; i < 1<<uint(d); i++ {
			// Don't need to mask off lower bits outside the valid ones because we
			// know they're already zero.
			path[0] = byte(i << uint(8-d))
			sfx := newSuffix(byte(d), path)
			// As an extra check there should be no collisions in the suffix values
			// that we build so map entries should not be overwritten.
			k := key{depth: uint8(d), value: path[0]}
			if _, ok := fromRaw[k]; ok {
				panic(fmt.Errorf("cache collision for: %v", k))
			}
			fromRaw[k] = sfx
			if _, ok := fromString[sfx.String()]; ok {
				panic(fmt.Errorf("cache collision for: %s", sfx.String()))
			}
			fromString[sfx.String()] = sfx
		}
	}
}

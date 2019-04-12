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

package storage

import (
	"encoding/base64"
)

var (
	// EmptySuffix is a reusable suffix of zero bits.
	EmptySuffix = NewSuffix(0, []byte{0})
	// byteToSuffix maps a single byte suffix (8 bits length) to a Suffix.
	byteToSuffix = make(map[byte]*Suffix)
	// strToSuffix maps a base64 encoded string representation to a Suffix.
	strToSuffix = make(map[string]*Suffix)
)

// Suffix represents the tail of a NodeID. It is the path within the subtree.
// The portion of the path that extends beyond the subtree is not part of this suffix.
// We keep a cache of the one byte Suffix values use by log trees. These
// are reused to avoid constant reallocation and base64 conversion overhead.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	// TODO(gdbelvin): make bits an integer.
	bits byte
	// path is the suffix itself.
	path []byte
	// asString is the string representation of the suffix.
	asString string
}

// NewSuffix creates a new Suffix. The primary use for them is to get their
// String value to use as a key so we compute that once up front.
func NewSuffix(bits byte, path []byte) *Suffix {
	// Use a shared value for a short suffix if we have one, they're immutable.
	if bits == 8 {
		if sfx, ok := byteToSuffix[path[0]]; ok {
			return sfx
		}
	}

	r := make([]byte, 1, len(path)+1)
	r[0] = bits
	r = append(r, path...)
	s := base64.StdEncoding.EncodeToString(r)

	return &Suffix{bits: bits, path: path, asString: s}
}

// Bits returns the number of significant bits in the Suffix path.
func (s Suffix) Bits() byte {
	return s.bits
}

// Path returns a copy of the Suffix path.
func (s Suffix) Path() []byte {
	return append(make([]byte, 0, len(s.path)), s.path...)
}

// String returns a string that represents Suffix.
// This is a base64 encoding of the following format:
// [ 1 byte for depth || path bytes ]
func (s Suffix) String() string {
	return s.asString
}

// ParseSuffix converts a suffix string back into a Suffix.
func ParseSuffix(s string) (*Suffix, error) {
	if sfx, ok := strToSuffix[s]; ok {
		// Matches a precalculated value, use that.
		return sfx, nil
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	return NewSuffix(byte(b[0]), b[1:]), nil
}

// Precalculate all the one byte suffix values so they can be reused
// either on construction or parsing.
func init() {
	for b := 0; b < 256; b++ {
		sfx := NewSuffix(8, []byte{byte(b)})
		byteToSuffix[byte(b)] = sfx
		strToSuffix[sfx.asString] = sfx
	}
}

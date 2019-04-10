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

var EmptySuffix = Suffix{path: []byte{0}}

// Suffix represents the tail of a NodeID. It is the path within the subtree.
// The portion of the path that extends beyond the subtree is not part of this suffix.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	// TODO(gdbelvin): make bits an integer.
	bits byte
	// path is the suffix itself.
	path []byte
	// asString is the string representation of the suffix.
	asString string
}

// NewSuffix creates a new Suffix. The only real point of using them is
// to get their String value so we compute that once up front.
func NewSuffix(bits byte, path []byte) *Suffix {
	r := make([]byte, 1, 1+(bits/8))
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
	return append([]byte(nil), s.path...)
}

// String returns a string that represents Suffix.
// This is a base64 encoding of the following format:
// [ 1 byte for depth || path bytes ]
func (s Suffix) String() string {
	return s.asString
}

// ParseSuffix converts a suffix string back into a Suffix.
func ParseSuffix(s string) (Suffix, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return EmptySuffix, err
	}

	return *NewSuffix(byte(b[0]), b[1:]), nil
}

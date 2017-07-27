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

// Suffix represents the tail of a NodeID. It is the path within the subtree.
// The portion of the path that extends beyond the subtree is not part of this suffix.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	// TODO(gdbelvin): make bits an integer.
	Bits byte
	// path is the suffix itself.
	Path []byte
}

// String returns a string that represents Suffix.
// This is a base64 encoding of the following format:
// [ 1 byte for depth || path bytes ]
func (s Suffix) String() string {
	r := make([]byte, 1, 1+(len(s.Path)))
	r[0] = s.Bits
	r = append(r, s.Path...)
	return base64.StdEncoding.EncodeToString(r)
}

// ParseSuffix converts a suffix string back into a Suffix.
func ParseSuffix(s string) (Suffix, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Suffix{}, err
	}

	return Suffix{
		Bits: byte(b[0]),
		Path: b[1:],
	}, nil
}

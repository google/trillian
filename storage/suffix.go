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
	"fmt"
)

// Suffix represents the tail of a NodeID, indexing into the Subtree which
// corresponds to the prefix of the NodeID.
type Suffix struct {
	// bits is the number of bits in the node ID suffix.
	// TODO(gdbelvin): make bits an integer.
	Bits byte
	// path is the suffix itself.
	Path []byte
}

// Serialize returns a base64 encoding of a byte array with the following format:
// [ 1 byte for depth || path bytes ]
func (s Suffix) Serialize() string {
	r := make([]byte, 1, 1+(len(s.Path)))
	r[0] = s.Bits
	r = append(r, s.Path...)
	return base64.StdEncoding.EncodeToString(r)
}

// makeSuffixKey creates a suffix key for indexing into the subtree's Leaves and
// InternalNodes maps.
// TODO(gdbelvin): deprecate in favor of nodeID.Split()
func makeSuffixKey(depth int, index int64) (string, error) {
	if depth < 0 {
		return "", fmt.Errorf("invalid negative depth of %d", depth)
	}
	if index < 0 {
		return "", fmt.Errorf("invalid negative index %d", index)
	}
	sfx := Suffix{byte(depth), []byte{byte(index)}}
	return sfx.Serialize(), nil
}

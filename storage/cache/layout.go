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

package cache

import (
	"encoding/binary"

	"github.com/google/trillian/merkle/compact"
)

// getTileID returns the path from the "virtual" root at level 64 to the root
// of the tile that the given node belongs to. All the bits of the returned
// slice are significant because all tile heights are 8.
//
// Note that a root of a tile belongs to a tile above it (as its leaf node).
// The exception is the "virtual" root which belongs to its own "pseudo" tile.
func getTileID(id compact.NodeID) []byte {
	if id.Level >= 64 {
		return []byte{} // Note: Not nil, so that storage/SQL doesn't use NULL.
	}
	index := id.Index >> (8 - id.Level%8)
	bytesCount := (64 - id.Level - 1) / 8

	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], index)
	return bytes[8-bytesCount:]
}

// splitID returns the path from the "virtual" root at level 64 to the root of
// the tile that the given node belongs to, and the corresponding local address
// of this node within this tile.
func splitID(id compact.NodeID) ([]byte, *Suffix) {
	if id.Level >= 64 {
		return []byte{}, emptySuffix
	}
	tileID := getTileID(id)

	var bytes [8]byte
	bits := 64 - id.Level - uint(len(tileID)*8)
	binary.BigEndian.PutUint64(bytes[:], id.Index<<(64-bits))
	suffix := NewSuffix(uint8(bits), bytes[:])

	return tileID, suffix
}

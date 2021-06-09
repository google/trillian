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

package tree

import (
	"encoding/binary"

	"github.com/google/trillian/merkle/compact"
)

// Layout defines the mapping between log tree node IDs and tile IDs.
type Layout struct{}

// GetTileID returns the path from the tree root to the tile that the given
// node belongs to. All the bits of the returned byte slice are used, because
// Layout enforces tile heights to be multiples of 8.
//
// Note that nodes located at strata boundaries normally belong to tiles rooted
// above them. However, the topmost node is the root for its own tile since
// there is nothing above it.
func (l Layout) GetTileID(id compact.NodeID) []byte {
	if id.Level >= 64 {
		return []byte{} // Note: Not nil, so that storage/SQL doesn't use NULL.
	}

	index := id.Index >> (8 - id.Level%8)
	bytesCount := (64 - id.Level - 1) / 8

	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], index)
	return bytes[8-bytesCount:]
}

// Split returns the path from the tree root to the tile that the given node
// belongs to, and the corresponding local address within this tile.
func (l Layout) Split(id compact.NodeID) ([]byte, *Suffix) {
	if id.Level >= 64 {
		return []byte{}, EmptySuffix
	}
	tileID := l.GetTileID(id)

	var bytes [8]byte
	bits := 64 - id.Level - uint(len(tileID)*8)
	binary.BigEndian.PutUint64(bytes[:], id.Index<<(64-bits))
	suffix := NewSuffix(uint8(bits), bytes[:])

	return tileID, suffix
}

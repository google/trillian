// Copyright 2019 Google Inc. All Rights Reserved.
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
	"errors"

	"github.com/google/trillian/storage/storagepb"
)

// TileSet contains an append-only set of tree tiles.
type TileSet struct {
	tiles map[string]*storagepb.SubtreeProto
}

// NewTileSet returns a new empty TileSet.
func NewTileSet() *TileSet {
	return &TileSet{tiles: make(map[string]*storagepb.SubtreeProto)}
}

// Touch marks the passed-in tile ID as touched. Returns true iff it was not
// touched before. Note that a tile can be touched but still not present, and
// so the corresponding Get call can return nil.
func (ts *TileSet) Touch(id TileID) bool {
	key := id.AsBytes()
	if _, found := ts.tiles[string(key)]; found {
		return false
	}
	ts.tiles[string(key)] = nil // We use nil value as an emptiness indicator.
	return true
}

// Add puts the passed-in tile to the TileSet. Returns an error if the tile
// structure is invalid, or it has been already added. An added tile is also
// marked as touched.
func (ts *TileSet) Add(sp *storagepb.SubtreeProto) error {
	if sp == nil {
		return errors.New("tile is nil")
	}
	if ts.tiles[string(sp.Prefix)] != nil {
		return errors.New("tile already exists")
	}
	ts.tiles[string(sp.Prefix)] = sp
	return nil
}

// Get returns the tile with the passed-in ID, or nil if it's not found.
func (ts *TileSet) Get(id TileID) *storagepb.SubtreeProto {
	return ts.tiles[id.AsKey()]
}

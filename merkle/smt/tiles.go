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

package smt

import (
	"bytes"
	"fmt"

	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/tree"
)

// TileSet represenst a set of Merkle tree tiles and the corresponding nodes.
//
// TODO(pavelkalinnikov): Make it immutable.
type TileSet struct {
	layout *tree.Layout
	tiles  map[tree.NodeID2][]Node
	hashes map[tree.NodeID2][]byte
	h      mapHasher
}

// NewTileSet creates an empty TileSet with the given tree parameters.
func NewTileSet(treeID int64, hasher hashers.MapHasher, layout *tree.Layout) *TileSet {
	tiles := make(map[tree.NodeID2][]Node)
	hashes := make(map[tree.NodeID2][]byte)
	h := bindHasher(hasher, treeID)
	return &TileSet{layout: layout, tiles: tiles, hashes: hashes, h: h}
}

// Hashes returns a map containing all node hashes keyed by node IDs.
func (t *TileSet) Hashes() map[tree.NodeID2][]byte {
	return t.hashes
}

// Add puts the given tile into the set.
//
// TODO(pavelkalinnikov): Take a whole list of Tiles instead.
func (t *TileSet) Add(tile Tile) error {
	if _, ok := t.tiles[tile.ID]; ok {
		return fmt.Errorf("tile already exists: %v", tile.ID)
	}
	t.tiles[tile.ID] = tile.Leaves
	return tile.scan(t.layout, t.h, func(node Node) {
		t.hashes[node.ID] = node.Hash
	})
}

// TileSetMutation accumulates tree tiles that need to be updated.
type TileSetMutation struct {
	read  *TileSet
	tiles map[tree.NodeID2][]Node
	dirty map[tree.NodeID2]bool
}

// NewTileSetMutation creates a mutation which is based off the provided
// TileSet. This means that each modification is checked against the hashes in
// this set, and is applied if it does change the hash.
func NewTileSetMutation(ts *TileSet) *TileSetMutation {
	tiles := make(map[tree.NodeID2][]Node)
	dirty := make(map[tree.NodeID2]bool)
	return &TileSetMutation{read: ts, tiles: tiles, dirty: dirty}
}

// Set updates the hash of the given tree node.
func (t *TileSetMutation) Set(id tree.NodeID2, hash []byte) {
	root := t.read.layout.GetTileRootID(id)
	height := uint(t.read.layout.TileHeight(int(root.BitLen())))
	if root.BitLen()+height != id.BitLen() {
		return // Not a leaf node of a tile.
	}
	if bytes.Equal(t.read.hashes[id], hash) {
		// TODO(pavelkalinnikov): Consider checking the dirty state.
		return // Nothing changed.
	}
	t.tiles[root] = append(t.tiles[root], Node{ID: id, Hash: hash})
	t.dirty[id] = true
}

// Build returns the full set of tiles modified by this mutation.
func (t *TileSetMutation) Build() ([]Tile, error) {
	res := make([]Tile, 0, len(t.tiles))
	for id, upd := range t.tiles {
		had, ok := t.read.tiles[id]
		if !ok {
			res = append(res, Tile{ID: id, Leaves: upd})
			continue
		}
		leaves := make([]Node, len(upd), len(upd)+len(had))
		copy(leaves, upd)
		for _, u := range had {
			if !t.dirty[u.ID] {
				leaves = append(leaves, u)
			}
		}
		// TODO(pavelkalinnikov): Introduce a Tile merge operation.
		// TODO(pavelkalinnikov): Sort leaves when the invariant is introduced.
		res = append(res, Tile{ID: id, Leaves: leaves})
	}
	return res, nil
}

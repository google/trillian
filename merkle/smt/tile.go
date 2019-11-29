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

import "github.com/google/trillian/storage/tree"

// Tile represents a sparse Merkle tree tile, i.e. a dense set of tree nodes
// located under a single "root" node at a distance not exceeding the tile
// height. A tile is identified by the ID of its root node. The Tile struct
// contains the list of non-empty leaf nodes, which can be used to reconstruct
// all the remaining inner nodes of the tile.
//
// TODO(pavelkalinnikov): Make Tile immutable.
// TODO(pavelkalinnikov): Introduce invariants on the order/content of Leaves.
type Tile struct {
	ID     tree.NodeID2
	Leaves []Node
}

// scan visits all non-empty nodes of the tile except the root. The order of
// node visits is arbitrary.
func (t Tile) scan(l *tree.Layout, h mapHasher, visit func(Node)) error {
	top := t.ID.BitLen()
	height := uint(l.TileHeight(int(top)))
	// TODO(pavelkalinnikov): Remove HStar3 side effects, to avoid copying here.
	// Currently, the Update method modifies the nodes given to NewHStar3.
	leaves := make([]Node, len(t.Leaves))
	copy(leaves, t.Leaves)
	hs, err := NewHStar3(leaves, h.mh.HashChildren, top+height, top)
	if err != nil {
		return err
	}
	_, err = hs.Update(emptyHashes{h: h, visit: visit})
	return err
}

// emptyHashes is a NodeAccessor used for computing node hashes of a tile.
type emptyHashes struct {
	h     mapHasher
	visit func(Node)
}

// Get returns an empty hash for the given root node ID.
func (e emptyHashes) Get(id tree.NodeID2) ([]byte, error) {
	return e.h.hashEmpty(id), nil
}

// Set calls the visitor callback for the given node and hash.
func (e emptyHashes) Set(id tree.NodeID2, hash []byte) {
	e.visit(Node{ID: id, Hash: hash})
}

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

package smt

import (
	"errors"
	"fmt"

	"github.com/google/trillian/storage/tree"
)

// Tile represents a sparse Merkle tree tile, i.e. a dense set of tree nodes
// located under a single "root" node at a distance not exceeding the tile
// height. A tile is identified by the ID of its root node. The Tile struct
// contains the list of non-empty leaf nodes, which can be used to reconstruct
// all the remaining inner nodes of the tile.
//
// Invariants of this structure that must be preserved at all times:
//  - ID is a prefix of Leaves' IDs, i.e. the nodes are in the same subtree.
//  - IDs of Leaves have the same length, i.e. the nodes are at the same level.
//  - Leaves are ordered by ID from left to right.
//  - IDs of Leaves are unique.
//
// Algorithms that create Tile structures must ensure that these invariants
// hold. Use NewNodesRow function for ordering nodes correctly.
type Tile struct {
	ID     tree.NodeID2
	Leaves NodesRow
}

// Merge returns a new tile which is a combination of this tile with the given
// updates. The resulting tile contains all the nodes from the updates, and all
// the nodes from the original tile not present in the updates.
func (t Tile) Merge(updates NodesRow) (Tile, error) {
	if len(updates) == 0 {
		return t, nil
	} else if len(t.Leaves) == 0 {
		return Tile{ID: t.ID, Leaves: updates}, nil
	}
	if at, want := updates[0].ID.BitLen(), t.Leaves[0].ID.BitLen(); at != want {
		return Tile{}, fmt.Errorf("updates are at depth %d, want %d", at, want)
	}
	if !updates.inSubtree(t.ID) {
		return Tile{}, errors.New("updates are not entirely in this tile")
	}
	return Tile{ID: t.ID, Leaves: merge(t.Leaves, updates)}, nil
}

// merge merges two sorted slices of nodes into one sorted slice. If a node ID
// exists in both slices, then the one from the updates slice is taken, i.e. it
// overrides the node from the nodes slice.
func merge(nodes, updates NodesRow) NodesRow {
	res := make([]Node, 0, len(nodes)+len(updates))
	i := 0
	for _, u := range updates {
		for ; i < len(nodes); i++ {
			if c := compareHorizontal(nodes[i].ID, u.ID); c < 0 {
				res = append(res, nodes[i])
			} else if c > 0 {
				break
			}
		}
		res = append(res, u)
	}
	return append(res, nodes[i:]...)
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

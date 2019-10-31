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
// TODO(pavelkalinnikov): Introduce invariants on the order/content of Leaves.
// TODO(pavelkalinnikov): Rename NodeUpdate to a more generic Node or NodeHash.
type Tile struct {
	ID     tree.NodeID2
	Leaves []NodeUpdate
}

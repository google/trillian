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

// Package tile includes functionality for describing a Merkle tree in
// terms of tiles and coordinates.
package tile

import (
	"bytes"
	"fmt"
)

// Coords represents the location of a single tile from a tiling of a Merkle tree.
// A tile with TileHeight of H and location (L, K) covers
//  - two nodes at the top of the tile (with height H.(L+1)-1)
//  - 2^H nodes at the bottom of the tile (with height H.L).
//
// For example, for H=2 the bottom-left tile boundaries are:
//
//   +---------------------------------------------------------------------------+
//   |                 3.0                                 3.1                   |
//   |       2.0                2.1               2.2                2.3         |
//   +---------------------------------------------------------------------------+
//   |  1.0      1.1  |   1.2      1.3  |   1.4       1.5   |   1.6      1.7     |
//   |0.0 0.1  0.2 0.3| 0.4 0.5  0.6 0.7| 0.8 0.9  0.10 0.11| 0.12 0.13 0.14 0.15|
//   +----------------+-----------------+-------------------+--------------------+
//
// notated as:
//   +----------------------------------------------+
//   |                  tile2(1,0)                  |
//   +----------------------+-----------------------+
//   |tile2(0,0)|tile2(0,1) | tile2(0,2)|tile2(0,2) |
//   +----------+-----------+-----------+-----------+
type Coords struct {
	Level, Index uint64
	TileHeight   uint64
}

func (tc Coords) String() string {
	return fmt.Sprintf("tile%d(%d,%d)", tc.TileHeight, tc.Level, tc.Index)
}

// Above returns the MerkleCoords of a single node above the tile, which encompasses
// the whole tile.
func (tc Coords) Above() MerkleCoords {
	return MerkleCoords{Level: tc.TileHeight * (tc.Level + 1), Index: tc.Index}
}

// Top returns the MerkleCoords of the two top nodes for the tile.
func (tc Coords) Top() [2]MerkleCoords {
	return [2]MerkleCoords{
		{Level: (tc.TileHeight * (tc.Level + 1)) - 1, Index: tc.Index * 2},
		{Level: (tc.TileHeight * (tc.Level + 1)) - 1, Index: tc.Index*2 + 1},
	}
}

// Bottom returns a slice (of size 2^tc.TileHeight) of MerkleCoords for the bottom
// nodes for the tile.
func (tc Coords) Bottom() []MerkleCoords {
	count := uint64(1) << tc.TileHeight
	results := make([]MerkleCoords, count)
	for idx := uint64(0); idx < count; idx++ {
		results[idx] = MerkleCoords{Level: tc.TileHeight * tc.Level, Index: (tc.Index * count) + idx}
	}
	return results
}

// Includes indicates whether the given coordinates are part of the tile.
func (tc Coords) Includes(c MerkleCoords) bool {
	// Allowed coordinate levels for this tile are [H.L, H.(L+1)).
	if c.Level < (tc.TileHeight*tc.Level) || c.Level >= (tc.TileHeight*(tc.Level+1)) {
		return false
	}
	return tc.Above().Contains(c)
}

// CoordsFor returns the tile coordinates for a tile that contains the given
// Merkle coordinates, for a tiling of a Merkle tree using tiles of the given
// height.
func CoordsFor(c MerkleCoords, tileHeight uint64) Coords {
	// The sublevel of a coordinate in the tile is in range [0, tileHeight),
	// and holds (2^tileHeight) / 2^sublevel index values.
	tileLevel := c.Level / tileHeight
	subLevel := c.Level % tileHeight
	levelWidth := uint64(1) << (tileHeight - subLevel)
	return Coords{TileHeight: tileHeight, Level: tileLevel, Index: c.Index / levelWidth}
}

// Tile represents a portion of a Merkle tree by encompassing a set of
// nodes and holding the hashes for the bottom nodes in the tile.
type Tile struct {
	Coords
	// Hashes holds the hash values associated with the bottom nodes for this tile.
	// There are at most 2^tileHeight of them, but there may be fewer for an
	// incomplete tile.
	Hashes [][]byte
	// hashChildren is used to calculate the hash of a node given the hashes of
	// its child nodes.
	hashChildren func(l, r []byte) []byte
}

// NewTile returns a new Tile structure for the given tile coordinates, including
// the given (non-empty) set of hashes.
func NewTile(coords Coords, hashes [][]byte, hasher func(l, r []byte) []byte) (*Tile, error) {
	tileWidth := 1 << coords.TileHeight
	if len(hashes) == 0 {
		return nil, fmt.Errorf("no hashes provided, want >=1 (and <=%d) of them", tileWidth)
	}
	if len(hashes) > tileWidth {
		return nil, fmt.Errorf("too many hashes provided (%d), need <= %d of them", len(hashes), tileWidth)
	}
	return &Tile{Coords: coords, Hashes: hashes, hashChildren: hasher}, nil
}

func (t *Tile) String() string {
	if len(t.Hashes) < (1 << t.TileHeight) {
		return fmt.Sprintf("%s/%d", t.Coords, len(t.Hashes))
	}
	return t.Coords.String()
}

// IsComplete indicates whether the tile is complete.
func (t *Tile) IsComplete() bool {
	return len(t.Hashes) == (1 << t.TileHeight)
}

// Extends indicates whether this tile is the same as another tile, only with
// more available hashes. A tile with the same number of available hashes is
// allowed.
func (t *Tile) Extends(precursor *Tile) error {
	if t.TileHeight != precursor.TileHeight {
		return fmt.Errorf("extension tile has different height %d than base %d", t.TileHeight, precursor.TileHeight)
	}
	if t.Level != precursor.Level || t.Index != precursor.Index {
		return fmt.Errorf("extension tile has different coords %v than base %v", t, precursor)
	}
	if len(t.Hashes) < len(precursor.Hashes) {
		return fmt.Errorf("extension tile has fewer hashes %d than base %d", len(t.Hashes), len(precursor.Hashes))
	}
	// Check common hashes are the same
	for i, hash := range precursor.Hashes {
		if !bytes.Equal(t.Hashes[i], hash) {
			return fmt.Errorf("extension tile has different hash [%d]=%x than base %x", i, t.Hashes[i], hash)
		}
	}
	return nil
}

// HashFor returns the Merkle tree hash associated with the given coordinates
// within the tile, provided that all of the constituent hashes are available.
func (t *Tile) HashFor(c MerkleCoords) ([]byte, error) {
	if c.IsPseudoNode() {
		return nil, fmt.Errorf("coordinates %v are for a pseudo-node", c)
	}
	if !t.Includes(c) {
		return nil, fmt.Errorf("coordinates %v not included in tile %v", t, c)
	}
	if c.Level == t.TileHeight*t.Level {
		// Lowest level nodes in tile are all level H*L, coordinate indices
		// [tileIndex*subtreeSize, (tileIndex+1)*subtreeSize)
		count := uint64(1) << t.TileHeight
		offset := c.Index - t.Index*count
		if int(offset) >= len(t.Hashes) {
			return nil, IncompleteTileError{Coords: t.Coords, Index: int(offset), Size: len(t.Hashes)}
		}
		return t.Hashes[offset], nil
	}
	// Ask for the right child first, so any incomplete tile error will encompass
	// both children.
	r, err := t.HashFor(c.RightChild())
	if err != nil {
		if err, ok := err.(IncompleteTileError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get right child hash for %v: %v", c, err)
	}
	l, err := t.HashFor(c.LeftChild())
	if err != nil {
		// If we got the right child, tile must be complete enough to include the
		// left child too, i.e. this error cannot be an IncompleteTileError.
		return nil, fmt.Errorf("failed to get left child hash for %v: %v", c, err)
	}
	return t.hashChildren(l, r), nil
}

// RootHash returns the Merkle tree hash that encompasses a complete tile (and
// so is associated with the coordinates t.Above()).  Fails for incomplete tiles.
func (t *Tile) RootHash() ([]byte, error) {
	if !t.IsComplete() {
		return nil, fmt.Errorf("root hash unavailable for incomplete tile %v", t)
	}
	top := t.Top()
	l, err := t.HashFor(top[0])
	if err != nil {
		// Unhittable as we've already checked for a complete tile.
		return nil, fmt.Errorf("failed to get top left hash for %v: %v", top[0], err)
	}
	r, err := t.HashFor(top[1])
	if err != nil {
		// Unhittable as we've already checked for a complete tile.
		return nil, fmt.Errorf("failed to get top right hash for %v: %v", top[1], err)
	}
	return t.hashChildren(l, r), nil
}

// IncompleteTileError indicates that a hash wasn't available in a tile because
// the tile did not contain enough hashes.
type IncompleteTileError struct {
	Coords      Coords
	Index, Size int
}

func (e IncompleteTileError) Error() string {
	return fmt.Sprintf("hash [%d] not available in incomplete tile %v with %d hashes", e.Index, e.Coords, e.Size)
}

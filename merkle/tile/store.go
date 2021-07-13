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

package tile

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/golang/glog"
)

// Store encapsulates the storage and retrieval of tiles covering
// a Merkle tree.  All uses of a given Store interface are assumed
// to use the same tileHeight value throughout.
type Store interface {
	TileHeight() uint64
	HashChildrenFn() func(l, r []byte) []byte
	// TileAt returns the tile for the given coordinates.  If the tile is
	// absent from the store, an error of type MissingTileError will be
	// returned.
	TileAt(tileCoords Coords) (*Tile, error)
	Add(tile *Tile) error
}

// MissingTileError indicates that a tile wasn't available in a Store.
type MissingTileError struct {
	Coords Coords
}

func (e MissingTileError) Error() string {
	return fmt.Sprintf("tile %v not available", e.Coords)
}

// RequiredTilesError indicates that hash retrieval failed because specific
// tiles were missing or incomplete
type RequiredTilesError struct {
	Missing    []MissingTileError
	Incomplete []IncompleteTileError
}

func (r RequiredTilesError) Error() string {
	var buf bytes.Buffer
	buf.WriteString("tiles required:")
	if len(r.Missing) > 0 {
		buf.WriteString(" missing:")
		for i, err := range r.Missing {
			if i == 0 {
				buf.WriteString(" ")
			} else {
				buf.WriteString(",")
			}
			buf.WriteString(err.Coords.String())
		}
	}
	if len(r.Incomplete) > 0 {
		buf.WriteString(" incomplete:")
		for i, err := range r.Incomplete {
			if i == 0 {
				buf.WriteString(" ")
			} else {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("%s/%d-need-/%d", err.Coords, err.Size, err.Index+1))
		}
	}
	return buf.String()
}

// combineErrs combines two errors, coping when one or both are instances of RequiredTilesError.
func combineErrs(l, r error) error {
	if l != nil {
		if l, ok := l.(RequiredTilesError); ok {
			if r == nil {
				return l
			}
			if r, ok := r.(RequiredTilesError); ok {
				return RequiredTilesError{
					Missing:    append(l.Missing, r.Missing...),
					Incomplete: append(l.Incomplete, r.Incomplete...),
				}
			}
			return r
		}
		return l // l is a real error, return it
	}
	return r
}

// HashFromStore attempts to find the hash for a specific node in the Merkle tree.
// If a relevant tile is missing or incomplete, an error of type RequiredTilesError
// will be returned.
func HashFromStore(store Store, node MerkleCoords) ([]byte, error) {
	if node.IsPseudoNode() {
		lNode := node.LeftChild()
		lHash, lErr := HashFromStore(store, lNode)
		rNode := node.RightChild()
		rHash, rErr := HashFromStore(store, rNode)
		if err := combineErrs(lErr, rErr); err != nil {
			return nil, err
		}
		hash := store.HashChildrenFn()(lHash, rHash)
		glog.V(4).Infof("HashFromStore(%v) = %x = hash(%x @ %v || %x @ %v)", node, hash[:8], lHash[:8], lNode, rHash[:8], rNode)
		return hash, nil
	}
	tileCoords := CoordsFor(node, store.TileHeight())
	tile, err := store.TileAt(tileCoords)
	if err != nil {
		if err, ok := err.(MissingTileError); ok {
			return nil, RequiredTilesError{Missing: []MissingTileError{err}}
		}
		return nil, fmt.Errorf("failed to get tile %v: %v", tileCoords, err)
	}
	hash, err := tile.HashFor(node)
	if err != nil {
		if err, ok := err.(IncompleteTileError); ok {
			return nil, RequiredTilesError{Incomplete: []IncompleteTileError{err}}
		}
		return nil, fmt.Errorf("failed to get hash for %v in %v: %v", node, tile, err)
	}
	glog.V(4).Infof("HashFromStore(%v) = %x from tile %v", node, hash[:8], tile)
	return hash, nil
}

func proofFromPath(store Store, path []MerkleCoords) ([][]byte, error) {
	// Accumulate missing and incomplete tiles.
	var proof [][]byte
	for i, node := range path {
		hash, err := HashFromStore(store, node)
		if err != nil {
			return nil, err
		}
		glog.V(4).Infof("proof[%d]=%x for %v", i, hash[:8], node)
		proof = append(proof, hash)
	}
	return proof, nil
}

// InclusionProofFromStore generates an inclusion proof for the given leaf index
// in a Merkle tree of the given size by retrieving data from a Store.  If
// the relevant tiles are unavailable, the returned error will be of type
// RequiredTilesError.
func InclusionProofFromStore(store Store, index uint64, treeSize uint64) ([][]byte, error) {
	return proofFromPath(store, InclusionPath(index, treeSize))
}

// ConsistencyProofFromStore returns a consistency proof between tree sizes,
// following the algorithm from [RFC6962] section 2.1.2, using the data in a
// Store.  If the relevant tiles are unavailable, the returned error will be of
// type RequiredTilesError.
func ConsistencyProofFromStore(store Store, fromSize, toSize uint64) ([][]byte, error) {
	return proofFromPath(store, ConsistencyPath(fromSize, toSize))
}

// MemoryStore is a Store implementation that just holds tile contents in
// memory, mainly for testing.
type MemoryStore struct {
	tileHeight   uint64
	hashChildren func(l, r []byte) []byte
	mu           sync.RWMutex
	tile         map[Coords]*Tile
}

// NewMemoryStore builds a MemoryStore for a tiling of the given height.
func NewMemoryStore(tileHeight uint64, hashChildren func(l, r []byte) []byte) Store {
	if tileHeight < 2 || tileHeight > MaxTreeHeight {
		panic(fmt.Sprintf("invalid tile height %d", tileHeight))
	}
	return &MemoryStore{
		hashChildren: hashChildren,
		tileHeight:   tileHeight,
		tile:         make(map[Coords]*Tile),
	}
}

// HashChildrenFn returns the function used to combine sub-node hashes.
func (m *MemoryStore) HashChildrenFn() func(l, r []byte) []byte {
	return m.hashChildren
}

// TileHeight returns the tile height.
func (m *MemoryStore) TileHeight() uint64 {
	return m.tileHeight
}

// TileAt returns the tile with the given coordinates, if available.
func (m *MemoryStore) TileAt(tileCoords Coords) (*Tile, error) {
	if tileCoords.TileHeight != m.tileHeight {
		return nil, fmt.Errorf("invalid tile height %d, only height %d supported", tileCoords.TileHeight, m.tileHeight)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	tile, ok := m.tile[tileCoords]
	if !ok {
		return nil, MissingTileError{Coords: tileCoords}
	}
	return tile, nil
}

// Add adds a tile to the in-memory storage.  This operation will fail if there
// is already a tile in the store that is inconsistent with the new tile.
func (m *MemoryStore) Add(tile *Tile) error {
	if tile.TileHeight != m.tileHeight {
		return fmt.Errorf("invalid tile height %d, only height %d supported", tile.TileHeight, m.tileHeight)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.tile[tile.Coords]; ok {
		if err := tile.Extends(existing); err != nil {
			return fmt.Errorf("tile %v contradicts existing tile %v: %v", tile, existing, err)
		}
	}
	m.tile[tile.Coords] = tile
	return nil
}

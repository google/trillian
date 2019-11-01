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

import "fmt"

const (
	// depthQuantum defines the smallest supported tile height, which all tile
	// heights must also be a multiple of.
	//
	// WARNING: The Layout type breaks if this value is not a multiple of 8,
	// because it uses NodeID byte representation directly.
	depthQuantum = 8
)

// Layout defines the mapping between tree node IDs and tile IDs.
type Layout struct {
	// sIndex contains stratum info for each multiple-of-depthQuantum node depth.
	// Note that if a stratum spans multiple depthQuantum heights then it will be
	// present in this slice the corresponding number of times.
	// This index is used for fast mapping from node IDs to strata IDs.
	sIndex []stratumInfo
	// Height is the height of the tree. It defines the maximal bit-length of a
	// node ID that the tree can contain.
	Height int
}

// NewLayout creates a tree layout based on the passed-in strata heights.
func NewLayout(heights []int) *Layout {
	// Compute the total tree height.
	height := 0
	for i, h := range heights {
		// Verify the stratum height is valid.
		if h <= 0 {
			panic(fmt.Errorf("invalid stratum height[%d]: %d; must be > 0", i, h))
		}
		if h%depthQuantum != 0 {
			panic(fmt.Errorf("invalid stratum height[%d]: %d; must be a multiple of %d", i, h, depthQuantum))
		}
		height += h
	}

	// Build the strata information index.
	sIndex := make([]stratumInfo, 0, height/depthQuantum)
	for _, h := range heights {
		// Assign the same stratum info to depth quants that this stratum spans.
		info := stratumInfo{idBytes: len(sIndex), height: h}
		for d := 0; d < h; d += depthQuantum {
			sIndex = append(sIndex, info)
		}
	}

	return &Layout{sIndex: sIndex, Height: height}
}

// GetTileID returns the ID of the tile that the given node belongs to.
//
// Note that nodes located at strata boundaries normally belong to tiles rooted
// above them. However, the topmost node (with an empty NodeID) is the root for
// its own tile since there is nothing above it.
func (l *Layout) GetTileID(id NodeID) TileID {
	if depth := id.PrefixLenBits; depth > 0 {
		info := l.getStratumAt(depth - 1)
		// TODO(pavelkalinnikov): Use Prefix method once it no longer copies Path.
		// TODO(pavelkalinnikov): Rename *FromHash to something sensible.
		root := NewNodeIDFromHash(id.Path[:info.idBytes])
		return TileID{Root: root}
	}
	// TODO(pavelkalinnikov): Leave Path == nil when it's safe.
	return TileID{Root: NodeID{Path: []byte{}}}
}

// Split returns the ID of the that the given node belongs to, and the
// corresponding local address within this tile.
func (l *Layout) Split(id NodeID) (TileID, *Suffix) {
	if depth := id.PrefixLenBits; depth > 0 {
		info := l.getStratumAt(depth - 1)
		root := NewNodeIDFromHash(id.Path[:info.idBytes])
		suffix := id.Suffix(info.idBytes, info.height)
		return TileID{Root: root}, suffix
	}
	// TODO(pavelkalinnikov): Leave Path == nil when it's safe.
	return TileID{Root: NodeID{Path: []byte{}}}, EmptySuffix
}

// TileHeight returns the height of a tile with its root located at the
// specified depth from the tree root. The result is not defined if rootDepth
// is not a tile boundary.
func (l *Layout) TileHeight(rootDepth int) int {
	return l.getStratumAt(rootDepth).height
}

func (l *Layout) getStratumAt(depth int) stratumInfo {
	return l.sIndex[depth/depthQuantum]
}

// stratumInfo describes a single stratum across the tree.
type stratumInfo struct {
	// idBytes is the byte length of IDs for this stratum.
	idBytes int
	// height is the number of tree levels in this stratum.
	height int
}

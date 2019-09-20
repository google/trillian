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

package cache

import (
	"fmt"

	"github.com/google/trillian/storage/tree"
)

const (
	// depthQuantum defines the smallest supported tile height, which all tile
	// heights must also be a multiple of.
	//
	// WARNING: The treeLayout type breaks if this value is not a multiple of 8,
	// because it uses tree.NodeID byte representation directly.
	depthQuantum = 8
)

// treeLayout defines the mapping between tree node IDs and tile IDs.
//
// TODO(pavelkalinnikov): Move this to storage/tree package, and rename to
// Layout, so that it's used in code as tree.Layout.
type treeLayout struct {
	// sIndex contains stratum info for each multiple-of-depthQuantum node depth.
	// Note that if a stratum spans multiple depthQuantum heights then it will be
	// present in this slice the corresponding number of times.
	// This index is used for fast mapping from node IDs to strata IDs.
	sIndex []stratumInfo
	// height is the height of the tree. It defines the maximal bit-length of a
	// node ID that the tree can contain.
	height int
}

// newTreeLayout creates a tree layout based on the passed-in strata heights.
func newTreeLayout(heights []int) *treeLayout {
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

	return &treeLayout{sIndex: sIndex, height: height}
}

// getTileID returns the ID of the tile that the given node belongs to.
//
// Note that nodes located at strata boundaries normally belong to tiles rooted
// above them. However, the topmost node (with an empty NodeID) is the root for
// its own tile since there is nothing above it.
func (t *treeLayout) getTileID(id tree.NodeID) tree.TileID {
	if depth := id.PrefixLenBits; depth > 0 {
		info := t.getStratumAt(depth - 1)
		// TODO(pavelkalinnikov): Use Prefix method once it no longer copies Path.
		// TODO(pavelkalinnikov): Rename *FromHash to something sensible.
		root := tree.NewNodeIDFromHash(id.Path[:info.idBytes])
		return tree.TileID{Root: root}
	}
	// TODO(pavelkalinnikov): Leave Path == nil when it's safe.
	return tree.TileID{Root: tree.NodeID{Path: []byte{}}}
}

// split returns the ID of the that the given node belongs to, and the
// corresponding local address within this tile.
func (t *treeLayout) split(id tree.NodeID) (tree.TileID, *tree.Suffix) {
	if depth := id.PrefixLenBits; depth > 0 {
		info := t.getStratumAt(depth - 1)
		root := tree.NewNodeIDFromHash(id.Path[:info.idBytes])
		suffix := id.Suffix(info.idBytes, info.height)
		return tree.TileID{Root: root}, suffix
	}
	return tree.TileID{Root: tree.NodeID{Path: []byte{}}}, tree.EmptySuffix
}

// getTileHeight returns the height of the tile with the passed-in ID.
func (t *treeLayout) getTileHeight(id tree.TileID) int {
	return t.getStratumAt(id.Root.PrefixLenBits).height
}

func (t *treeLayout) getStratumAt(depth int) stratumInfo {
	return t.sIndex[depth/depthQuantum]
}

// stratumInfo describes a single stratum across the tree.
type stratumInfo struct {
	// idBytes is the byte length of IDs for this stratum.
	idBytes int
	// height is the number of tree levels in this stratum.
	height int
}

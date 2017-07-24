// Copyright 2017 Google Inc. All Rights Reserved.
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

package node

import "bytes"

//
// Transition Plan
//
// 1. Migrate use of PrefixLenBits to PrefixForDepth function.
//

//
// Differences between Log and Map paths
// - Both are constant sized byte slices.
//
// - Log paths grow in significance from LSB to MSB.
// - Map paths grow in signifiance from MSB to LSB.

// Node represents a location in a merkle tree, formed by a path and a depth.
// The root of a tree is indicated by depth 0.
// Leaves are stored at depth == len(path)*8
type Node struct {
	// Path is left-aligned. Nodes with depths that are not multiples of 8
	// will have unused bits in the least significant bit positions.
	path []byte

	// Number of significant bits in path, starting from MSB.
	// Depth 0 means the root node, a path "" of length zero bits.
	depth int
}

// New returns a node defined by a full path.
// For a map, this is a leaf node.
// For a log, this is also a leaf node.
func New(index []byte) *Node {
	return &Node{
		path:  index,
		depth: len(index) * 8,
	}
}

// Copy returns a copy of Node.
func (n *Node) Copy() *Node {
	p := make([]byte, len(n.path))
	copy(p, n.path)
	return &Node{
		path:  p,
		depth: n.depth,
	}
}

// Equal returns true iff paths are byte for byte equal (including unused bytes)
// and they have equal depths.
func Equal(a, b *Node) bool {
	return bytes.Equal(a.path, b.path) && a.depth == b.depth
}

// PathBits returns the maximum number of bits in path, including ignored bits.
// PathBits returns multiples of 8.
// For maps, this is the height of the full tree.
// For logs, this is the height of the current tree. XXX: what does that mean?
func (n *Node) PathBits() int {
	return len(n.path) * 8
}

// LeftBit returns the ith bit from MSB.
func (n *Node) LeftBit(i int) uint {
	panic("unimplemented")
}

// RightBit returns the ith bit from LSB.
// eg. RightBit(0x80000000, 31) -> 1
func (n *Node) RightBit(i int) uint {
	// TODO: assert i < PathBits()?
	bIndex := (n.PathBits() - i - 1) / 8
	return uint((n.path[bIndex] >> uint(i%8)) & 0x01)
}

// FlipRightBit flips the i'th bit from the right.
func (n *Node) FlipRightBit(i int) *Node {
	bIndex := (n.PathBits() - i - 1) / 8
	n.path[bIndex] ^= 1 << uint(i%8)
	return n
}

// leftmask contains bitmasks indexed such that the left x bits are set. It is
// indexed by byte position from 0-7 0 is special cased to 0xFF since 8 mod 8
// is 0. leftmask is only used to mask the last byte.
var leftmask = [8]byte{0xFF, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFC, 0xFE}

// MaskLeft keeps depth bits from MSB and zeros everything else.
// It also sets depth to depth.
func (n *Node) MaskLeft(depth int) *Node {
	r := make([]byte, len(n.path))
	if depth > 0 {
		// Copy the first depthBytes.
		depthBytes := (depth + 7) >> 3
		copy(r, n.path[:depthBytes])
		// Mask off unwanted bits in the last byte.
		r[depthBytes-1] = r[depthBytes-1] & leftmask[depth%8]
	}

	n.depth = depth
	n.path = r
	return n
}

// Neighbors returns the neighbors of this node, starting in order from LSB to MSB.
func (n *Node) Neighbors() []*Node {
	r := make([]*Node, 0, n.depth)
	for i := n.depth; i > 0; i-- {
		r = append(r, n.Copy().FlipBit(i).MaskLeft(i))
	}
}

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

package compact

import "math/bits"

// NodeID identifies a node of a Merkle tree.
//
// The level is the longest distance from the node down to the leaves, and
// index is its horizontal position in this level ordered from left to right.
// Consider an example below where nodes are labeled as [<level> <index>].
//
//           [2 0]
//          /     \
//       [1 0]     \
//       /   \      \
//   [0 0]  [0 1]  [0 2]
type NodeID struct {
	Level uint
	Index uint64
}

// NewNodeID returns a NodeID with the passed in node coordinates.
func NewNodeID(level uint, index uint64) NodeID {
	return NodeID{Level: level, Index: index}
}

// RangeNodes returns node IDs that comprise the [begin, end) compact range.
func RangeNodes(begin, end uint64) []NodeID {
	left, right := Decompose(begin, end)
	ids := make([]NodeID, 0, bits.OnesCount64(left)+bits.OnesCount64(right))

	pos := begin
	// Iterate over perfect subtrees along the left border of the range, ordered
	// from lower to upper levels.
	for bit := uint64(0); left != 0; pos, left = pos+bit, left^bit {
		level := uint(bits.TrailingZeros64(left))
		bit = uint64(1) << level
		ids = append(ids, NewNodeID(level, pos>>level))
	}

	// Iterate over perfect subtrees along the right border of the range, ordered
	// from upper to lower levels.
	for bit := uint64(0); right != 0; pos, right = pos+bit, right^bit {
		level := uint(bits.Len64(right)) - 1
		bit = uint64(1) << level
		ids = append(ids, NewNodeID(level, pos>>level))
	}

	return ids
}

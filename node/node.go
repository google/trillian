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

// PathBits returns the maximum number of bits in path, including ignored bits.
// PathBits currently returns multiples of 8.
// For maps, this is the height of the full tree.
// For logs, this is the height of the current tree.
func (n *Node) PathBits() int {
	return len(n.path) * 8
}

// PrefixBits returns the number of bits from path that form the prefix.
// PrefixBits is computed deterministically from depth.
func (n *Node) PrefixBits() int {
	panic("unimplemented")
	return -1
}

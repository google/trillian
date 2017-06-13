// Copyright 2016 Google Inc. All Rights Reserved.
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

package merkle

// MapHasher computes hashes for leafs, empty branches, and intermediate nodes for
// a SparseMerkleTree.
type MapHasher interface {
	// HashEmpty returns the hash of an empty branch at a given depth.
	// A depth of 0 indictes the root of an empty tree.
	// A depth of Size*8 + 1 indicates an empty leaf.
	HashEmpty(depth int) []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) []byte
	// HashChildren computes interior nodes.
	// TODO(gbelvin) rename to HashInterior.
	HashChildren(l, r []byte) []byte
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

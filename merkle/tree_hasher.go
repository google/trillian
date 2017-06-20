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

// TreeHasher provides the hash functions needed to compute both log and sparse merkle trees.
type TreeHasher interface {
	// EmptyRoot supports returning a special case for the root of an empty tree.
	EmptyRoot() []byte
	// HashEmpty returns the hash of an empty branch at a given depth.
	// A height of 0 indicates an empty leaf. The maximum height is Size*8.
	HashEmpty(height int) []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) []byte
	// HashChildren computes interior nodes.
	HashChildren(l, r []byte) []byte
	// Size is the number of bits in the underlying hash function.
	// It is also the maximum height of the merkle tree.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

// Copyright 2016 Google LLC. All Rights Reserved.
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

package hashers

// LogHasher provides the hash functions needed to compute dense merkle trees.
type LogHasher interface {
	// EmptyRoot supports returning a special case for the root of an empty tree.
	EmptyRoot() []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) []byte
	// HashChildren computes interior nodes.
	HashChildren(l, r []byte) []byte
	// Size is the number of bytes in the underlying hash function.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

// MapHasher provides the hash functions needed to compute sparse merkle trees.
type MapHasher interface {
	// HashEmpty returns the hash of an empty subtree of the given height. The
	// location of the subtree root is defined by the index parameter, which
	// encodes the path from the tree root to the node as a bit sequence of
	// length BitLen()-height. Note that a height of 0 indicates a leaf.
	//
	// Each byte of index is considered from MSB to LSB. The last byte might be
	// used partially, but the unused bits must be zero. If index contains more
	// bytes than necessary then the remaining bytes are ignored. May panic if
	// the index is too short for the specified height.
	//
	// TODO(pavelkalinnikov): Pass in NodeID2 which type-safely defines index and
	// height/depth simultaneously.
	HashEmpty(treeID int64, index []byte, height int) []byte
	// HashLeaf computes the hash of a leaf that exists.  This method
	// is *not* used for computing the hash of a leaf that does not exist
	// (instead, HashEmpty(treeID, index, 0) is used), as the hash value
	// can be different between:
	//  - a leaf that is unset
	//  - a leaf that has been explicitly set, including set to []byte{}.
	HashLeaf(treeID int64, index []byte, leaf []byte) []byte
	// HashChildren computes interior nodes, when at least one of the child
	// subtrees is non-empty.
	HashChildren(l, r []byte) []byte
	// Size is the number of bytes in the underlying hash function.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
	// BitLen returns the number of bits in the underlying hash function.
	// It is also the height of the merkle tree.
	BitLen() int
}

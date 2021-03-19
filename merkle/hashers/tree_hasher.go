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

import "github.com/google/trillian/storage/tree"

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
	// HashEmpty returns the hash of an empty subtree with the given root. Note
	// that the empty NodeID2 indicates the root of the entire tree.
	HashEmpty(treeID int64, root tree.NodeID2) []byte
	// HashLeaf computes the hash of an existing leaf. Note that for non-existing
	// leaves the HashEmpty method must be used instead, because we differentiate
	// unset leaves and leaves that are set to an empty byte string.
	HashLeaf(treeID int64, id tree.NodeID2, leaf []byte) []byte
	// HashChildren computes interior nodes, when at least one of the child
	// subtrees is non-empty.
	HashChildren(l, r []byte) []byte
}

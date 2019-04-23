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

package hashers

import (
	"fmt"

	"github.com/google/trillian"
)

// LogHasher provides the hash functions needed to compute dense merkle trees.
type LogHasher interface {
	// EmptyRoot supports returning a special case for the root of an empty tree.
	EmptyRoot() []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) ([]byte, error)
	// HashLeafInto computes the hash of a leaf into an existing slice,
	// which is mutated.
	// Note: Implementations are not expected to be thread safe. The caller must
	// ensure that concurrent calls are not made and is responsible for making
	// copies of slices where necessary to avoid data being overwritten.
	HashLeafInto(leaf, res []byte) ([]byte, error)
	// HashChildren computes interior nodes.
	HashChildren(l, r []byte) []byte
	// HashChildrenInto computes interior nodes into an existing slice, which is
	// mutated.
	// Note: Implementations are not expected to be thread safe. The caller must
	// ensure that concurrent calls are not made and is responsible for making
	// copies of slices where necessary to avoid data being overwritten.
	HashChildrenInto(l, r, res []byte) []byte
	// Size is the number of bytes in the underlying hash function.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

// MapHasher provides the hash functions needed to compute sparse merkle trees.
type MapHasher interface {
	// HashEmpty returns the hash of an empty branch at a given depth.
	// A height of 0 indicates an empty leaf. The maximum height is Size*8.
	// TODO(gbelvin) fully define index.
	HashEmpty(treeID int64, index []byte, height int) []byte
	// HashLeaf computes the hash of a leaf that exists.  This method
	// is *not* used for computing the hash of a leaf that does not exist
	// (instead, HashEmpty(treeID, index, 0) is used), as the hash value
	// can be different between:
	//  - a leaf that is unset
	//  - a leaf that has been explicitly set, including set to []byte{}.
	HashLeaf(treeID int64, index []byte, leaf []byte) ([]byte, error)
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

type NewHasherFunc func() LogHasher

var (
	logHashers = make(map[trillian.HashStrategy]NewHasherFunc)
	mapHashers = make(map[trillian.HashStrategy]MapHasher)
)

// RegisterLogHasher registers a hasher for use.
func RegisterLogHasher(h trillian.HashStrategy, f NewHasherFunc) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterLogHasher(%s) of unknown hasher", h))
	}
	if logHashers[h] != nil {
		panic(fmt.Sprintf("%v already registered as a LogHasher", h))
	}
	logHashers[h] = f
}

// RegisterMapHasher registers a hasher for use.
func RegisterMapHasher(h trillian.HashStrategy, f MapHasher) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterMapHasher(%s) of unknown hasher", h))
	}
	if mapHashers[h] != nil {
		panic(fmt.Sprintf("%v already registered as a MapHasher", h))
	}
	mapHashers[h] = f
}

// NewLogHasher returns a LogHasher.
// TODO(Martin2112): The name of this func implies it creates a new instance
// but it doesn't.
func NewLogHasher(h trillian.HashStrategy) (LogHasher, error) {
	f := logHashers[h]
	if f != nil {
		return f(), nil
	}
	return nil, fmt.Errorf("LogHasher(%s) is an unknown hasher", h)
}

// NewMapHasher returns a MapHasher.
// TODO(Martin2112): The name of this func implies it creates a new instance
// but it doesn't.
func NewMapHasher(h trillian.HashStrategy) (MapHasher, error) {
	f := mapHashers[h]
	if f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("MapHasher(%s) is an unknown hasher", h)
}

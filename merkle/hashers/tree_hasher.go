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

// LogHasher provides the hash functions needed to compute dense merkele trees.
type LogHasher interface {
	// EmptyRoot supports returning a special case for the root of an empty tree.
	EmptyRoot() []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) []byte
	// HashChildren computes interior nodes.
	HashChildren(l, r []byte) []byte
	// Size is the number of bits in the underlying hash function.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

// MapHasher provides the hash functions needed to compute sparse merkle trees.
// TODO(gbelvin): Update interface to match #670
type MapHasher interface {
	// HashEmpty returns the hash of an empty branch at a given depth.
	// A height of 0 indicates an empty leaf. The maximum height is Size*8.
	HashEmpty(height int) []byte
	// HashLeaf computes the hash of a leaf that exists.
	HashLeaf(leaf []byte) []byte
	// HashChildren computes interior nodes.
	HashChildren(l, r []byte) []byte
	// Size is the number of bits in the underlying hash function.
	// It is also the height of the merkle tree.
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

var (
	maxHashers = trillian.HashStrategy(len(trillian.HashStrategy_value))
	logHashers = make([]LogHasher, maxHashers)
	mapHashers = make([]MapHasher, maxHashers)
)

// RegisterLogHasher registers a hasher for use.
func RegisterLogHasher(h trillian.HashStrategy, f LogHasher) {
	if h < 0 || h >= maxHashers {
		panic(fmt.Sprintf("RegisterLogHasher(%v) of unknown hasher", h))
	}
	logHashers[h] = f
}

// RegisterMapHasher registers a hasher for use.
func RegisterMapHasher(h trillian.HashStrategy, f MapHasher) {
	if h < 0 || h >= maxHashers {
		panic(fmt.Sprintf("RegisterMapHasher(%v) of unknown hasher", h))
	}
	mapHashers[h] = f
}

// NewLogHasher returns a LogHasher
func NewLogHasher(h trillian.HashStrategy) (LogHasher, error) {
	if h >= 0 || h < maxHashers {
		h := logHashers[h]
		if h != nil {
			return h, nil
		}
	}
	return nil, fmt.Errorf("NewLogHasher(%v) is unknown hasher", h)
}

// NewMapHasher returns a MapHasher
func NewMapHasher(h trillian.HashStrategy) (MapHasher, error) {
	if h >= 0 || h < maxHashers {
		h := mapHashers[h]
		if h != nil {
			return h, nil
		}
	}
	return nil, fmt.Errorf("NewMapHasher(%v) is unknown hasher", h)
}

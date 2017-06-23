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
	maxHashers = maxKey(trillian.HashStrategy_name) + 1
	logHashers = make([]LogHasher, maxHashers)
	mapHashers = make([]MapHasher, maxHashers)
)

func maxKey(m map[int32]string) int32 {
	var max int32
	for k := range m {
		if k > max {
			max = k
		}
	}
	return max
}

// RegisterLogHasher registers a hasher for use.
func RegisterLogHasher(h trillian.HashStrategy, f LogHasher) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterLogHasher(%v) of unknown hasher", h))
	}
	logHashers[h] = f
}

// RegisterMapHasher registers a hasher for use.
func RegisterMapHasher(h trillian.HashStrategy, f MapHasher) {
	if h == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY {
		panic(fmt.Sprintf("RegisterMapHasher(%v) of unknown hasher", h))
	}
	mapHashers[h] = f
}

// GetLogHasher returns a LogHasher
func GetLogHasher(h trillian.HashStrategy) (LogHasher, error) {
	f := logHashers[h]
	if f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("LogHasher(%v) is unknown hasher", h)
}

// GetMapHasher returns a MapHasher
func GetMapHasher(h trillian.HashStrategy) (MapHasher, error) {
	f := mapHashers[h]
	if f != nil {
		return f, nil
	}
	return nil, fmt.Errorf("MapHasher(%v) is unknown hasher", h)
}

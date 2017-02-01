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

// MapHasher is a specialised TreeHasher which also knows about the set of
// "null" hashes for the unused sections of a SparseMerkleTree.
type MapHasher struct {
	TreeHasher
	nullHashes [][]byte
}

// NewMapHasher creates a new MapHasher based on the passed in hash function.
func NewMapHasher(th TreeHasher) MapHasher {
	return MapHasher{
		TreeHasher: th,
		nullHashes: createNullHashes(th),
	}
}

func createNullHashes(th TreeHasher) [][]byte {
	numEntries := th.Size() * 8
	r := make([][]byte, numEntries, numEntries)
	r[numEntries-1] = th.HashLeaf([]byte{})
	for i := numEntries - 2; i >= 0; i-- {
		r[i] = th.HashChildren(r[i+1], r[i+1])
	}
	return r
}

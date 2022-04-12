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

// Package inmemory provides an in-memory MerkleTree implementation.
// It's a fairly direct port of the C++ Merkle Tree from the CT repo; it has the same API
// and should have similar performance.
// It is not part of the Trillian API.
//
// Note: this implementation evaluates the root lazily in the same way as the C++ code so
// some methods that appear to be accessors can cause mutations to update the structure
// to the necessary point required to obtain the result.
//
// -------------------------------------------------------------------------------------------
// IMPORTANT NOTE: This code uses 1-based leaf indexing as this is how the original C++
// works. There is scope for confusion if it is mixed with the Trillian specific trees in
// this package, which index leaves starting from zero. This code is primarily meant for use in
// cross checks of the new implementation and it is advantageous to be able to compare it
// directly with the C++ code.
// -------------------------------------------------------------------------------------------
package inmemory

import (
	"github.com/transparency-dev/merkle"
)

// MerkleTree holds a Merkle Tree in memory.
type MerkleTree struct {
	impl *Tree
}

// NewMerkleTree creates a new empty Merkle Tree using the specified Hasher.
func NewMerkleTree(hasher merkle.LogHasher) *MerkleTree {
	return &MerkleTree{impl: New(hasher)}
}

// LeafHash returns the hash of the requested leaf, or nil if it doesn't exist.
func (mt *MerkleTree) LeafHash(leaf int64) []byte {
	if leaf == 0 || leaf > mt.LeafCount() {
		return nil
	}
	return mt.impl.n[0][leaf-1]
}

// LeafCount returns the number of leaves in the tree.
func (mt *MerkleTree) LeafCount() int64 {
	return int64(mt.impl.Size())
}

// AddLeaf adds a new leaf to the hash tree. Stores the hash of the leaf data in the
// tree structure, does not store the data itself.
//
// (We will evaluate the tree lazily, and not update the root here.)
//
// Returns the position of the leaf in the tree. Indexing starts at 1,
// so position = number of leaves in the tree after this update.
func (mt *MerkleTree) AddLeaf(leafData []byte) (int64, []byte) {
	leafHash := mt.impl.h.HashLeaf(leafData)
	return mt.addLeafHash(leafHash)
}

func (mt *MerkleTree) addLeafHash(hash []byte) (int64, []byte) {
	mt.impl.Append(hash)
	return int64(mt.impl.Size()), hash
}

// CurrentRoot set the current root of the tree.
// Updates the root to reflect the current shape of the tree and returns the tree digest.
//
// Returns the hash of an empty string if the tree has no leaves
// (and hence, no root).
func (mt *MerkleTree) CurrentRoot() []byte {
	return mt.RootAtSnapshot(mt.LeafCount())
}

// RootAtSnapshot gets the root of the tree for a previous snapshot,
// where snapshot 0 is an empty tree, snapshot 1 is the tree with
// 1 leaf, etc.
//
// Returns an empty string if the snapshot requested is in the future
// (i.e., the tree is not large enough).
func (mt *MerkleTree) RootAtSnapshot(snapshot int64) []byte {
	if uint64(snapshot) > mt.impl.Size() {
		return nil
	}
	hash, err := mt.impl.HashAt(uint64(snapshot))
	if err != nil {
		panic(err)
	}
	return hash
}

// PathToCurrentRoot get the Merkle path from leaf to root for a given leaf.
//
// Returns a slice of node hashes, ordered by levels from leaf to root.
// The first element is the sibling of the leaf hash, and the last element
// is one below the root.
// Returns an empty slice if the tree is not large enough
// or the leaf index is 0.
func (mt *MerkleTree) PathToCurrentRoot(leaf int64) [][]byte {
	return mt.PathToRootAtSnapshot(leaf, mt.LeafCount())
}

// PathToRootAtSnapshot gets the Merkle path from a leaf to the root for a previous snapshot.
//
// Returns a slice of node hashes, ordered by levels from leaf to
// root.  The first element is the sibling of the leaf hash, and the
// last element is one below the root.  Returns an empty slice if
// the leaf index is 0, the snapshot requested is in the future or
// the snapshot tree is not large enough.
func (mt *MerkleTree) PathToRootAtSnapshot(leaf int64, snapshot int64) [][]byte {
	if leaf > snapshot || snapshot > mt.LeafCount() || leaf == 0 {
		return nil
	}
	hashes, err := mt.impl.InclusionProof(uint64(leaf-1), uint64(snapshot))
	if err != nil {
		panic(err)
	}
	return hashes
}

// SnapshotConsistency gets the Merkle consistency proof between two snapshots.
// Returns a slice of node hashes, ordered according to levels.
// Returns an empty slice if snapshot1 is 0, snapshot1 >= snapshot2,
// or one of the snapshots requested is in the future.
func (mt *MerkleTree) SnapshotConsistency(snapshot1 int64, snapshot2 int64) [][]byte {
	if snapshot1 == 0 || snapshot1 >= snapshot2 || snapshot2 > mt.LeafCount() {
		return nil
	}
	hashes, err := mt.impl.ConsistencyProof(uint64(snapshot1), uint64(snapshot2))
	if err != nil {
		panic(err)
	}
	return hashes
}

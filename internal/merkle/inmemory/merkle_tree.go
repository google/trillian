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
	"fmt"

	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/proof"
)

// TreeEntry is used for nodes in the tree for better readability. Just holds a hash but could be extended
type TreeEntry struct {
	hash []byte
}

// Hash returns the current hash in a newly created byte slice that the caller owns and may modify.
func (t TreeEntry) Hash() []byte {
	var newSlice []byte

	return t.HashInto(newSlice)
}

// HashInto returns the current hash in a provided byte slice that the caller
// may use to make multiple calls to obtain hashes without reallocating memory.
func (t TreeEntry) HashInto(dest []byte) []byte {
	dest = dest[:0] // reuse the existing space

	dest = append(dest, t.hash...)
	return dest
}

// TreeEntryDescriptor wraps a node and is used to describe tree paths, which are useful to have
// access to when testing the code and examining how it works
type TreeEntryDescriptor struct {
	Value  TreeEntry
	XCoord int64 // The horizontal node coordinate
	YCoord int64 // The vertical node coordinate
}

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

// NodeCount gets the current node count (of the lazily evaluated tree).
// Caller is responsible for keeping track of the lazy evaluation status. This will not
// update the tree.
func (mt *MerkleTree) NodeCount(level int64) int64 {
	if levels := mt.LevelCount(); levels <= level {
		panic(fmt.Errorf("LevelCount <= level in nodeCount: %d", levels))
	}
	return int64(len(mt.impl.n[level]))
}

// LevelCount returns the number of levels in the Merkle tree.
func (mt *MerkleTree) LevelCount() int64 {
	cnt := int64(len(mt.impl.n))
	if leaves := mt.LeafCount(); leaves&(leaves-1) != 0 {
		cnt++
	}
	return cnt
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
func (mt *MerkleTree) AddLeaf(leafData []byte) (int64, TreeEntry) {
	leafHash := mt.impl.h.HashLeaf(leafData)
	return mt.addLeafHash(leafHash)
}

func (mt *MerkleTree) addLeafHash(hash []byte) (int64, TreeEntry) {
	mt.impl.Append(hash)
	return int64(mt.impl.Size()), TreeEntry{hash: hash}
}

// CurrentRoot set the current root of the tree.
// Updates the root to reflect the current shape of the tree and returns the tree digest.
//
// Returns the hash of an empty string if the tree has no leaves
// (and hence, no root).
func (mt *MerkleTree) CurrentRoot() TreeEntry {
	return mt.RootAtSnapshot(mt.LeafCount())
}

// RootAtSnapshot gets the root of the tree for a previous snapshot,
// where snapshot 0 is an empty tree, snapshot 1 is the tree with
// 1 leaf, etc.
//
// Returns an empty string if the snapshot requested is in the future
// (i.e., the tree is not large enough).
func (mt *MerkleTree) RootAtSnapshot(snapshot int64) TreeEntry {
	if uint64(snapshot) > mt.impl.Size() {
		return TreeEntry{}
	}
	hash, err := mt.impl.HashAt(uint64(snapshot))
	if err != nil {
		panic(err)
	}
	return TreeEntry{hash: hash}
}

// PathToCurrentRoot get the Merkle path from leaf to root for a given leaf.
//
// Returns a slice of node hashes, ordered by levels from leaf to root.
// The first element is the sibling of the leaf hash, and the last element
// is one below the root.
// Returns an empty slice if the tree is not large enough
// or the leaf index is 0.
func (mt *MerkleTree) PathToCurrentRoot(leaf int64) []TreeEntryDescriptor {
	return mt.PathToRootAtSnapshot(leaf, mt.LeafCount())
}

// PathToRootAtSnapshot gets the Merkle path from a leaf to the root for a previous snapshot.
//
// Returns a slice of node hashes, ordered by levels from leaf to
// root.  The first element is the sibling of the leaf hash, and the
// last element is one below the root.  Returns an empty slice if
// the leaf index is 0, the snapshot requested is in the future or
// the snapshot tree is not large enough.
func (mt *MerkleTree) PathToRootAtSnapshot(leaf int64, snapshot int64) []TreeEntryDescriptor {
	if leaf > snapshot || snapshot > mt.LeafCount() || leaf == 0 {
		return []TreeEntryDescriptor{}
	}

	nodes, err := proof.Inclusion(uint64(leaf-1), uint64(snapshot))
	if err != nil {
		panic(err)
	}
	hashes, err := mt.impl.InclusionProof(uint64(leaf-1), uint64(snapshot))
	if err != nil {
		panic(err)
	}
	return mt.mojo(nodes, hashes)
}

// SnapshotConsistency gets the Merkle consistency proof between two snapshots.
// Returns a slice of node hashes, ordered according to levels.
// Returns an empty slice if snapshot1 is 0, snapshot1 >= snapshot2,
// or one of the snapshots requested is in the future.
func (mt *MerkleTree) SnapshotConsistency(snapshot1 int64, snapshot2 int64) []TreeEntryDescriptor {
	if snapshot1 == 0 || snapshot1 >= snapshot2 || snapshot2 > mt.LeafCount() {
		return nil
	}

	nodes, err := proof.Consistency(uint64(snapshot1), uint64(snapshot2))
	if err != nil {
		panic(err)
	}
	hashes, err := mt.impl.ConsistencyProof(uint64(snapshot1), uint64(snapshot2))
	if err != nil {
		panic(err)
	}
	return mt.mojo(nodes, hashes)
}

func (mt *MerkleTree) mojo(nodes proof.Nodes, hashes [][]byte) []TreeEntryDescriptor {
	ephem, begin, end := nodes.Ephem()
	res := make([]TreeEntryDescriptor, len(hashes))
	idx := 0
	for i, h := range hashes {
		id := nodes.IDs[idx]
		if idx >= begin && idx < end && begin+1 < end {
			id = ephem
			idx = end - 1
		}
		idx++

		res[i] = TreeEntryDescriptor{
			Value:  TreeEntry{hash: h},
			YCoord: int64(id.Level),
			XCoord: int64(id.Index),
		}
	}
	return res
}

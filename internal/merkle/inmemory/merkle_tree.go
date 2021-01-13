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
	"errors"
	"fmt"

	"github.com/google/trillian/merkle/hashers"
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

// MerkleTree holds a Merkle Tree in memory as a 2D node array
type MerkleTree struct {
	// A container for nodes, organized according to levels and sorted
	// left-to-right in each level. tree_[0] is the leaf level, etc.
	// The hash of nodes tree_[i][j] and tree_[i][j+1] (j even) is stored
	// at tree_[i+1][j/2]. When tree_[i][j] is the last node of the level with
	// no right sibling, we store its dummy copy: tree_[i+1][j/2] = tree_[i][j].
	//
	// For example, a tree with 5 leaf hashes a0, a1, a2, a3, a4
	//
	//        __ hash__
	//       |         |
	//    __ h20__     a4
	//   |        |
	//  h10     h11
	//  | |     | |
	// a0 a1   a2 a3
	//
	// is internally represented, top-down
	//
	// --------
	// | hash |                        tree_[3]
	// --------------
	// | h20  | a4  |                  tree_[2]
	// -------------------
	// | h10  | h11 | a4 |             tree_[1]
	// -----------------------------
	// | a0   | a1  | a2 | a3 | a4 |   tree_[0]
	// -----------------------------
	//
	// Since the tree is append-only from the right, at any given point in time,
	// at each level, all nodes computed so far, except possibly the last node,
	// are fixed and will no longer change.
	tree            [][]TreeEntry
	leavesProcessed int64
	levelCount      int64
	hasher          hashers.LogHasher
}

// isPowerOfTwoPlusOne tests whether a number is (2^x)-1 for some x. From MerkleTreeMath in C++
func isPowerOfTwoPlusOne(leafCount int64) bool {
	if leafCount == 0 {
		return false
	}

	if leafCount == 1 {
		return true
	}
	// leaf_count is a power of two plus one if and only if
	// ((leaf_count - 1) & (leaf_count - 2)) has no bits set.
	return ((leafCount - 1) & (leafCount - 2)) == 0
}

// sibling returns the index of the node's (left or right) sibling in the same level.
func sibling(leaf int64) int64 {
	if isRightChild(leaf) {
		return leaf - 1
	}
	return leaf + 1
}

// NewMerkleTree creates a new empty Merkle Tree using the specified Hasher.
func NewMerkleTree(hasher hashers.LogHasher) *MerkleTree {
	return &MerkleTree{hasher: hasher}
}

// LeafHash returns the hash of the requested leaf, or nil if it doesn't exist.
func (mt *MerkleTree) LeafHash(leaf int64) []byte {
	if leaf == 0 || leaf > mt.LeafCount() {
		return nil
	}
	return mt.tree[0][leaf-1].hash
}

// NodeCount gets the current node count (of the lazily evaluated tree).
// Caller is responsible for keeping track of the lazy evaluation status. This will not
// update the tree.
func (mt *MerkleTree) NodeCount(level int64) int64 {
	if mt.lazyLevelCount() <= level {
		panic(fmt.Errorf("lazyLevelCount <= level in nodeCount: %d", mt.lazyLevelCount()))
	}

	return int64(len(mt.tree[level]))
}

// LevelCount returns the number of levels in the Merkle tree.
func (mt *MerkleTree) LevelCount() int64 {
	return mt.levelCount
}

// lazyLevelCount is the current level count of the lazily evaluated tree.
func (mt *MerkleTree) lazyLevelCount() int64 {
	return int64(len(mt.tree))
}

// LeafCount returns the number of leaves in the tree.
func (mt *MerkleTree) LeafCount() int64 {
	if len(mt.tree) == 0 {
		return 0
	}
	return mt.NodeCount(0)
}

// root gets the current root (of the lazily evaluated tree).
// Caller is responsible for keeping track of the lazy evaluation status.
func (mt *MerkleTree) root() TreeEntry {
	lastLevel := len(mt.tree) - 1

	if len(mt.tree[lastLevel]) > 1 {
		panic(fmt.Errorf("found multiple nodes in root: %d", len(mt.tree[lastLevel])))
	}

	return mt.tree[lastLevel][0]
}

// lastNode returns the last node of the given level in the tree.
func (mt *MerkleTree) lastNode(level int64) TreeEntry {
	levelNodes := mt.NodeCount(level)

	if levelNodes < 1 {
		panic(fmt.Errorf("no nodes at level %d in lastNode", level))
	}

	return mt.tree[level][levelNodes-1]
}

// addLevel start a new tree level.
func (mt *MerkleTree) addLevel() {
	mt.tree = append(mt.tree, []TreeEntry{})
}

// pushBack appends a node to the level.
func (mt *MerkleTree) pushBack(level int64, treeEntry TreeEntry) {
	if mt.lazyLevelCount() <= level {
		panic(fmt.Errorf("lazyLevelCount <= level in pushBack: %d", mt.lazyLevelCount()))
	}

	mt.tree[level] = append(mt.tree[level], treeEntry)
}

// popBack pops (removes and returns) the last node of the level.
func (mt *MerkleTree) popBack(level int64) {
	if len(mt.tree[level]) < 1 {
		panic(errors.New("no nodes to pop in popBack"))
	}

	mt.tree[level] = mt.tree[level][:len(mt.tree[level])-1]
}

// AddLeaf adds a new leaf to the hash tree. Stores the hash of the leaf data in the
// tree structure, does not store the data itself.
//
// (We will evaluate the tree lazily, and not update the root here.)
//
// Returns the position of the leaf in the tree. Indexing starts at 1,
// so position = number of leaves in the tree after this update.
func (mt *MerkleTree) AddLeaf(leafData []byte) (int64, TreeEntry) {
	leafHash := mt.hasher.HashLeaf(leafData)
	leafCount, treeEntry := mt.addLeafHash(leafHash)
	return leafCount, treeEntry
}

func (mt *MerkleTree) addLeafHash(leafData []byte) (int64, TreeEntry) {
	treeEntry := TreeEntry{}
	treeEntry.hash = leafData

	if mt.lazyLevelCount() == 0 {
		// The first leaf hash is also the first root.
		mt.addLevel()
		mt.leavesProcessed = 1
	}

	mt.pushBack(0, treeEntry)
	leafCount := mt.LeafCount()

	// Update level count: a k-level tree can hold 2^{k-1} leaves,
	// so increment level count every time we overflow a power of two.
	// Do not update the root; we evaluate the tree lazily.
	if isPowerOfTwoPlusOne(leafCount) {
		mt.levelCount++
	}

	return leafCount, treeEntry
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
	if snapshot == 0 {
		return TreeEntry{mt.hasher.EmptyRoot()}
	}

	// Snapshot index bigger than tree, this is not the TreeEntry you're looking for
	if snapshot > mt.LeafCount() {
		return TreeEntry{nil}
	}

	if snapshot >= mt.leavesProcessed {
		return mt.updateToSnapshot(snapshot)
	}

	// snapshot < leaves_processed_: recompute the snapshot root.
	return mt.recomputePastSnapshot(snapshot, 0, nil)
}

// updateToSnapshot updates the tree to a given snapshot (if necessary), returns the root.
func (mt *MerkleTree) updateToSnapshot(snapshot int64) TreeEntry {
	if snapshot == 0 {
		return TreeEntry{mt.hasher.EmptyRoot()}
	}

	if snapshot == 1 {
		return mt.tree[0][0]
	}

	if snapshot == mt.leavesProcessed {
		return mt.root()
	}

	if snapshot > mt.LeafCount() {
		panic(errors.New("snapshot size > leaf count in updateToSnapshot"))
	}

	if snapshot <= mt.leavesProcessed {
		panic(errors.New("snapshot size <= leavesProcessed in updateToSnapshot"))
	}

	// Update tree, moving up level-by-level.
	level := int64(0)
	// Index of the first node to be processed at the current level.
	firstNode := mt.leavesProcessed
	// Index of the last node.
	lastNode := snapshot - 1

	// Process level-by-level until we converge to a single node.
	// (first_node, last_node) = (0, 0) means we have reached the root level.
	for lastNode != 0 {
		if mt.lazyLevelCount() <= level+1 {
			mt.addLevel()
		} else if mt.NodeCount(level+1) == parent(firstNode)+1 {
			// The leftmost parent at level 'level+1' may already exist,
			// so we need to update it. Nuke the old parent.
			mt.popBack(level + 1)
		}

		// Compute the parents of new nodes at the current level.
		// Start with a left sibling and parse an even number of nodes.
		for j := firstNode &^ 1; j < lastNode; j += 2 {
			mt.pushBack(level+1, TreeEntry{mt.hasher.HashChildren(mt.tree[level][j].hash, mt.tree[level][j+1].hash)})
		}

		// If the last node at the current level is a left sibling,
		// dummy-propagate it one level up.
		if !isRightChild(lastNode) {
			mt.pushBack(level+1, mt.tree[level][lastNode])
		}

		firstNode = parent(firstNode)
		lastNode = parent(lastNode)
		level++
	}

	mt.leavesProcessed = snapshot

	return mt.root()
}

// recomputePastSnapshot returns the root of the tree as it was for a past snapshot.
// If node is not nil, additionally records the rightmost node for the given snapshot and node_level.
func (mt *MerkleTree) recomputePastSnapshot(snapshot int64, nodeLevel int64, node *TreeEntry) TreeEntry {
	level := int64(0)
	// Index of the rightmost node at the current level for this snapshot.
	lastNode := snapshot - 1

	if snapshot == mt.leavesProcessed {
		// Nothing to recompute.
		if node != nil && mt.lazyLevelCount() > nodeLevel {
			if nodeLevel > 0 {
				*node = mt.lastNode(nodeLevel)
			} else {
				// Leaf level: grab the last processed leaf.
				*node = mt.tree[nodeLevel][lastNode]
			}
		}

		return mt.root()
	}

	if snapshot >= mt.leavesProcessed {
		panic(errors.New("snapshot size >= leavesProcessed in recomputePastSnapshot"))
	}

	// Recompute nodes on the path of the last leaf.
	for isRightChild(lastNode) {
		if node != nil && nodeLevel == level {
			*node = mt.tree[level][lastNode]
		}

		// Left sibling and parent exist in the snapshot, and are equal to
		// those in the tree; no need to rehash, move one level up.
		lastNode = parent(lastNode)
		level++
	}

	// Now last_node is the index of a left sibling with no right sibling.
	// Record the node.
	subtreeRoot := mt.tree[level][lastNode]

	if node != nil && nodeLevel == level {
		*node = subtreeRoot
	}

	for lastNode != 0 {
		if isRightChild(lastNode) {
			// Recompute the parent of tree_[level][last_node].
			subtreeRoot = TreeEntry{mt.hasher.HashChildren(mt.tree[level][lastNode-1].hash, subtreeRoot.hash)}
		}
		// Else the parent is a dummy copy of the current node; do nothing.

		lastNode = parent(lastNode)
		level++
		if node != nil && nodeLevel == level {
			*node = subtreeRoot
		}
	}

	return subtreeRoot
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

	return mt.pathFromNodeToRootAtSnapshot(leaf-1, 0, snapshot)
}

// pathFromNodeToRootAtSnapshot returns the path from a node at a given level
// (both indexed starting with 0) to the root at a given snapshot.
func (mt *MerkleTree) pathFromNodeToRootAtSnapshot(node int64, level int64, snapshot int64) []TreeEntryDescriptor {
	var path []TreeEntryDescriptor

	if snapshot == 0 {
		return path
	}

	// Index of the last node.
	lastNode := (snapshot - 1) >> uint64(level)

	if level >= mt.levelCount || node > lastNode || snapshot > mt.LeafCount() {
		return path
	}

	if snapshot > mt.leavesProcessed {
		// Bring the tree sufficiently up to date.
		mt.updateToSnapshot(snapshot)
	}

	// Move up, recording the sibling of the current node at each level.
	for lastNode != 0 {
		sibling := sibling(node)

		if sibling < lastNode {
			// The sibling is not the last node of the level in the snapshot
			// tree, so its value is correct in the tree.
			path = append(path, TreeEntryDescriptor{mt.tree[level][sibling], level, sibling})
		} else if sibling == lastNode {
			// The sibling is the last node of the level in the snapshot tree,
			// so we get its value for the snapshot. Get the root in the same pass.
			var recomputeNode TreeEntry

			mt.recomputePastSnapshot(snapshot, level, &recomputeNode)
			path = append(path, TreeEntryDescriptor{recomputeNode, -level, -sibling})
		}
		// Else sibling > last_node so the sibling does not exist. Do nothing.
		// Continue moving up in the tree, ignoring dummy copies.
		node = parent(node)
		lastNode = parent(lastNode)
		level++
	}

	return path
}

// SnapshotConsistency gets the Merkle consistency proof between two snapshots.
// Returns a slice of node hashes, ordered according to levels.
// Returns an empty slice if snapshot1 is 0, snapshot1 >= snapshot2,
// or one of the snapshots requested is in the future.
func (mt *MerkleTree) SnapshotConsistency(snapshot1 int64, snapshot2 int64) []TreeEntryDescriptor {
	var proof []TreeEntryDescriptor

	if snapshot1 == 0 || snapshot1 >= snapshot2 || snapshot2 > mt.LeafCount() {
		return proof
	}

	level := int64(0)
	// Rightmost node in snapshot1.
	node := snapshot1 - 1

	// Compute the (compressed) path to the root of snapshot2.
	// Everything left of 'node' is equal in both trees; no need to record.
	for isRightChild(node) {
		node = parent(node)
		level++
	}

	if snapshot2 > mt.leavesProcessed {
		// Bring the tree sufficiently up to date.
		mt.updateToSnapshot(snapshot2)
	}

	// Record the node, unless we already reached the root of snapshot1.
	if node != 0 {
		proof = append(proof, TreeEntryDescriptor{mt.tree[level][node], level, node})
	}

	// Now record the path from this node to the root of snapshot2.
	path := mt.pathFromNodeToRootAtSnapshot(node, level, snapshot2)

	return append(proof, path...)
}

// parent returns the index of the parent node in the parent level of the tree.
func parent(index int64) int64 {
	return index >> 1
}

// isRightChild returns true if the node is a right child.
func isRightChild(index int64) bool {
	return index&1 == 1
}

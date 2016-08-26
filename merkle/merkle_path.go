package merkle

import (
	"fmt"
	
	"github.com/google/trillian/storage"
)

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to
// build an inclusion proof for a specified leaf and tree size. The maxBitLen parameter
// is copied into all the returned nodeIDs.
func CalcInclusionProofNodeAddresses(treeSize, index int64, maxBitLen int) ([]storage.NodeID, error) {
	if index >= treeSize || index < 0 || treeSize < 1 || maxBitLen < 0 {
		return []storage.NodeID{}, fmt.Errorf("invalid params ts: %d index: %d, bitlen:%d", treeSize, index, maxBitLen)
	}

	proof := make([]storage.NodeID, 0, bitLen(treeSize) + 1)

	sizeLessOne := treeSize - 1

	if bitLen(treeSize) == 0 || index > sizeLessOne {
		return proof, nil
	}

	node := index
	depth := 0
	lastNodeAtLevel := sizeLessOne

	for depth < bitLen(sizeLessOne) {
		sibling := node ^ 1
		if sibling < lastNodeAtLevel {
			// Tree must be completely filled in up to this node index
			proof = append(proof, storage.NewNodeIDForTreeCoords(int64(depth), sibling, maxBitLen))
		} else if sibling == lastNodeAtLevel {
			// The tree may skip levels because it's not completely filled in. These nodes
			// don't exist
			drop := depth - subtreeDepth(treeSize, depth-1)
			sibling = sibling << uint(drop)
			proof = append(proof, storage.NewNodeIDForTreeCoords(int64(depth - drop), sibling, maxBitLen))
		}

		node = node >> 1
		lastNodeAtLevel = lastNodeAtLevel >> 1
		depth++
	}

	return proof, nil
}

// CalcConsistencyProofNodeAddresses returns the tree node IDs needed to
// build a consistency proof between two specified tree sizes. The maxBitLen parameter
// is copied into all the returned nodeIDs. The caller is responsible for checking that
// the input tree sizes correspond to valid tree heads. All returned NodeIDs are tree
// coordinates within the new tree. It is assumed that they will be fetched from storage
// at a revision corresponding to the STH associated with the treeSize parameter.
func CalcConsistencyProofNodeAddresses(previousTreeSize, treeSize int64, maxBitLen int) ([]storage.NodeID, error) {
	if previousTreeSize > treeSize || previousTreeSize < 1 || treeSize < 1 || maxBitLen <= 0 {
		return []storage.NodeID{}, fmt.Errorf("invalid params prior: %d treesize: %d, bitlen:%d", previousTreeSize, treeSize, maxBitLen)
	}

	return snapshotConsistency(previousTreeSize, treeSize, maxBitLen), nil
}

// snapshotConsistency does the calculation of consistency proof node addresses between
// two snapshots. Based on the C++ code used by CT but adjusted to fit our situation.
// In particular the code does not need to handle the case where overwritten node hashes
// must be recursively computed because we have versioned nodes.
func snapshotConsistency(snapshot1, snapshot2 int64, maxBitLen int) []storage.NodeID {
	proof := make([]storage.NodeID, 0, bitLen(snapshot2) + 1)

	level := 0
	node := snapshot1 - 1

	// Compute the (compressed) path to the root of snapshot2.
	// Everything left of 'node' is equal in both trees; no need to record.
	for (node & 1) != 0 {
		node >>= 1
		level++
	}

	if node != 0 {
		// Not at the root of snapshot 1, record the node
		proof = append(proof, storage.NewNodeIDForTreeCoords(int64(level), node, maxBitLen))
	}

	// Now append the path from this node to the root of snapshot2.
	return append(proof, pathFromNodeToRootAtSnapshot(node, level, snapshot2, maxBitLen)...)
}

func pathFromNodeToRootAtSnapshot(node int64, level int, snapshot int64, maxBitLen int) []storage.NodeID {
	proof := make([]storage.NodeID, 0, bitLen(snapshot) + 1)

	if snapshot == 0 {
		return proof
	}

	// Index of the last node.
	lastNode := (snapshot - 1) >> uint(level)

	// Move up, recording the sibling of the current node at each level.
	for lastNode != 0 {
		sibling := node ^ 1
		if sibling < lastNode {
			// The sibling is not the last node of the level in the snapshot tree
			proof = append(proof, storage.NewNodeIDForTreeCoords(int64(level), sibling, maxBitLen))
		} else if sibling == lastNode {
			// The sibling is the last node of the level in the snapshot tree.
			// In the C++ code we'd potentially recompute the node value here because we could be
			// referencing a snapshot at a point before additional leaves were added to the tree causing
			// some nodes to be overwritten. We have versioned tree nodes so this isn't necessary,
			// we won't see any hashes written since the snapshot point. However we do have to account
			// for missing levels in the tree.
			drop := level - subtreeDepth(snapshot, level - 1)
			sibling = sibling << uint(drop)
			proof = append(proof, storage.NewNodeIDForTreeCoords(int64(level - drop), sibling, maxBitLen))
		}

		// Sibling > lastNode so does not exist, move up
		node >>= 1
		lastNode >>= 1
		level++
	}

	return proof
}

// subtreeDepth calculates the depth of a subtree, used at the right of the tree which
// may not be completely populated
func subtreeDepth(size int64, bits int) int {
	for b := bitLen(size) - 1; b > bits; b-- {
		size = size &^ (1 << uint(b))
	}

	// determine tree height for the remaining bits.
	p2 := bitLen(size) - 1
	size = size &^ (1 << uint(p2))
	if bitLen(size) > 0 {
		p2++
	}

	return p2
}

package merkle

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/storage"
)

// Verbosity level for logging of debug related items
const vLevel = 2

// NodeFetch bundles a nodeID with additional information on how to use the node to construct the
// correct proof.
type NodeFetch struct {
	NodeID storage.NodeID
	Rehash bool
}

func (n NodeFetch) Equivalent(other NodeFetch) bool {
	return n.Rehash == other.Rehash && n.NodeID.Equivalent(other.NodeID)
}

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to
// build an inclusion proof for a specified leaf and tree size. The maxBitLen parameter
// is copied into all the returned nodeIDs.
func CalcInclusionProofNodeAddresses(treeSize, index int64, maxBitLen int) ([]NodeFetch, error) {
	if index >= treeSize || index < 0 || treeSize < 1 || maxBitLen < 0 {
		return []NodeFetch{}, fmt.Errorf("invalid params ts: %d index: %d, bitlen:%d", treeSize, index, maxBitLen)
	}

	proof := make([]NodeFetch, 0, bitLen(treeSize) + 1)

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
			n, err := storage.NewNodeIDForTreeCoords(int64(depth), sibling, maxBitLen)
			if err != nil {
				return nil, err
			}
			proof = append(proof, NodeFetch{NodeID:n})
		} else if sibling == lastNodeAtLevel {
			// We're working in the same node coordinate space as the C++ reference implementation
			// (depth, index) but intermediate nodes with only one child are not written by our storage.
			// In these cases the value that we want is a copy of a node further down (multiple levels
			// may be skipped).
			l, sibling := skipMissingLevels(treeSize, lastNodeAtLevel, depth, node)
			n, err := storage.NewNodeIDForTreeCoords(int64(l), sibling, maxBitLen)
			if err != nil {
				return nil, err
			}
			proof = append(proof, NodeFetch{NodeID:n})
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
func CalcConsistencyProofNodeAddresses(previousTreeSize, treeSize int64, maxBitLen int) ([]NodeFetch, error) {
	if previousTreeSize > treeSize || previousTreeSize < 1 || treeSize < 1 || maxBitLen <= 0 {
		return []NodeFetch{}, fmt.Errorf("invalid params prior: %d treesize: %d, bitlen:%d", previousTreeSize, treeSize, maxBitLen)
	}

	return snapshotConsistency(previousTreeSize, treeSize, maxBitLen)
}

// snapshotConsistency does the calculation of consistency proof node addresses between
// two snapshots. Based on the C++ code used by CT but adjusted to fit our situation.
// In particular the code does not need to handle the case where overwritten node hashes
// must be recursively computed because we have versioned nodes.
func snapshotConsistency(snapshot1, snapshot2 int64, maxBitLen int) ([]NodeFetch, error) {
	proof := make([]NodeFetch, 0, bitLen(snapshot2) + 1)

	glog.V(vLevel).Infof("snapshotConsistency: %d -> %d", snapshot1, snapshot2)

	level := 0
	node := snapshot1 - 1

	// Compute the (compressed) path to the root of snapshot2.
	// Everything left of 'node' is equal in both trees; no need to record.
	for (node & 1) != 0 {
		glog.V(vLevel).Infof("Move up: l:%d n:%d", level, node)
		node >>= 1
		level++
	}

	if node != 0 {
		glog.V(vLevel).Infof("Not root snapshot1: %d", node)
		// Not at the root of snapshot 1, record the node
		n, err := storage.NewNodeIDForTreeCoords(int64(level), node, maxBitLen)
		if err != nil {
			return nil, err
		}
		proof = append(proof, NodeFetch{NodeID:n})
	}

	// Now append the path from this node to the root of snapshot2.
	p, err := pathFromNodeToRootAtSnapshot(node, level, snapshot2, maxBitLen)
	if err != nil {
		return nil, err
	}
	return append(proof, p...), nil
}

func pathFromNodeToRootAtSnapshot(node int64, level int, snapshot int64, maxBitLen int) ([]NodeFetch, error) {
	glog.V(vLevel).Infof("pathFromNodeToRootAtSnapshot: N:%d, L:%d, S:%d", node, level, snapshot)
	proof := make([]NodeFetch, 0, bitLen(snapshot) + 1)

	if snapshot == 0 {
		return proof, nil
	}

	// Index of the last node (if the level is fully populated).
	lastNode := (snapshot - 1) >> uint(level)

	// Move up, recording the sibling of the current node at each level.
	for lastNode != 0 {
		sibling := node ^ 1
		if sibling < lastNode {
			// The sibling is not the last node of the level in the snapshot tree
			glog.V(vLevel).Infof("Not last: S:%d L:%d", sibling, level)
			n, err := storage.NewNodeIDForTreeCoords(int64(level), sibling, maxBitLen)
			if err != nil {
				return nil, err
			}
			proof = append(proof, NodeFetch{NodeID:n})
		} else if sibling == lastNode {
			// The sibling is the last node of the level in the snapshot tree.
			// In the C++ code we'd potentially recompute the node value here because we could be
			// referencing a snapshot at a point before additional leaves were added to the tree causing
			// some nodes to be overwritten. We have versioned tree nodes so this isn't necessary,
			// we won't see any hashes written since the snapshot point. However we do have to account
			// for missing levels in the tree. This can only occur on the rightmost tree nodes because
			// this is the only area of the tree that is not fully populated.
			glog.V(vLevel).Infof("Last: S:%d L:%d", sibling, level)

			// Account for non existent nodes - these can only be the rightmost node at an
			// intermediate (non leaf) level in the tree so will always be a right sibling.
			l, sibling := skipMissingLevels(snapshot, lastNode, level, node)
			n, err := storage.NewNodeIDForTreeCoords(int64(l), sibling, maxBitLen)
			if err != nil {
				return nil, err
			}
			proof = append(proof, NodeFetch{NodeID:n})
		} else {
			glog.V(vLevel).Infof("Nonexistent: S:%d L:%d", sibling, level)
		}

		// Sibling > lastNode so does not exist, move up
		node >>= 1
		lastNode >>= 1
		level++
	}

	return proof, nil
}

// lastNodeWritten determines if the last node is present in storage for a given Merkle tree size
// and level in the tree (0 = leaves, increasing towards the root). This is determined by
// examining the bits of the last valid leaf index in a tree of the specified size. Zero bits
// indicate nodes that are not stored at that tree size.
//
// Examples, all using a tree of size 5 leaves:
//
// As depicted in RFC 6962, nodes "float" upwards.
//
//            hash2
//            /  \
//           /    \
//          /      \
//         /        \
//        /          \
//        k            i
//       / \           |
//      /   \          e
//     /     \         |
//    g       h       d4
//   / \     / \
//   a b     c d
//   | |     | |
//   d0 d1   d2 d3
//
// In the C++ reference implementation, intermediate nodes are stored, leaves are at level 0.
// There is a dummy copy from the level below stored where the last node at a level has no right
// sibling. More detail is given in the comments of:
// https://github.com/google/certificate-transparency/blob/master/cpp/merkletree/merkle_tree.h
//
//             hash2
//             /  \
//            /    \
//           /      \
//          /        \
//         /          \
//        k            e
//       / \             \
//      /   \             \
//     /     \             \
//    g       h           e
//   / \     / \         /
//   a b     c d        e
//   | |     | |        |
//   d0 d1   d2 d3      d4
//
// In our storage implementation shown in the next diagram, nodes "sink" downwards, [X] nodes
// with one child are not written, there is no dummy copy. Leaves are at level zero.
//
//             hash2
//             /  \
//            /    \
//           /      \
//          /        \
//         /          \
//        k            [X]           Level 2
//       / \             \
//      /   \             \
//     /     \             \
//    g       h           [X]        Level 1
//   / \     / \         /
//   a b     c d        e            Level 0
//   | |     | |        |
//   d0 d1   d2 d3      d4
//
// Tree size = 5, last index = 4 in binary = 100, append 1 for leaves = 1001.
// Reading down the RHS: present, not present, not present, present = 1001. So when
// attempting to fetch the sibling of k (level 2, index 1) the tree should be descended twice to
// fetch 'e' (level 0, index 4) as (level 1, index 2) is also not present in storage.
func lastNodePresent(level, ts int64) bool {
	if level == 0 {
		// Leaves always exist
		return true
	}

	// Last index in the level is the tree size - 1
	bits := uint64(ts - 1)
	// Test the bit in the path for the requested level
	mask := uint64(1) << uint64(level-1)

	return bits&mask != 0
}

// skipMissingLevels moves down the tree a level towards the leaves until the node exists. This
// must terminate successfully as we will eventually reach the leaves, which are always written
// and are at level 0. Missing nodes are intermediate nodes with one child, hence their value
// is the same as the node lower down the tree as there is nothing to hash it with.
func skipMissingLevels(snapshot, lastNode int64, level int, node int64) (int, int64) {
	sibling := node ^ 1
	for level > 0 && sibling == lastNode && !lastNodePresent(int64(level), snapshot) {
		level--
		sibling *= 2
		lastNode = (snapshot - 1) >> uint(level)
		glog.V(vLevel).Infof("Move down: S:%d L:%d LN:%d", sibling, level, lastNode)
	}

	return level, sibling
}

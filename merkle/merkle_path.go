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

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/storage"
	"github.com/vektra/errors"
)

// Verbosity levels for logging of debug related items
const vLevel = 2
const vvLevel = 4

// NodeFetch bundles a nodeID with additional information on how to use the node to construct the
// correct proof.
type NodeFetch struct {
	NodeID storage.NodeID
	Rehash bool
}

// Equivalent return true iff the other represents the same rehash state and NodeID as the other.
func (n NodeFetch) Equivalent(other NodeFetch) bool {
	return n.Rehash == other.Rehash && n.NodeID.Equivalent(other.NodeID)
}

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to
// build an inclusion proof for a specified leaf and tree size. The maxBitLen parameter
// is copied into all the returned nodeIDs.
func CalcInclusionProofNodeAddresses(snapshot, index, treeSize int64, maxBitLen int) ([]NodeFetch, error) {
	if snapshot > treeSize || index >= snapshot || index < 0 || snapshot < 1 || maxBitLen < 0 {
		return []NodeFetch{}, fmt.Errorf("invalid params s: %d index: %d ts: %d, bitlen:%d", snapshot, index, treeSize, maxBitLen)
	}

	return pathFromNodeToRootAtSnapshot(index, 0, snapshot, treeSize, maxBitLen)
}

// CalcConsistencyProofNodeAddresses returns the tree node IDs needed to
// build a consistency proof between two specified tree sizes. The maxBitLen parameter
// is copied into all the returned nodeIDs. The caller is responsible for checking that
// the input tree sizes correspond to valid tree heads. All returned NodeIDs are tree
// coordinates within the new tree. It is assumed that they will be fetched from storage
// at a revision corresponding to the STH associated with the treeSize parameter.
func CalcConsistencyProofNodeAddresses(snapshot1, snapshot2, treeSize int64, maxBitLen int) ([]NodeFetch, error) {
	if snapshot1 > snapshot2 || snapshot1 > treeSize || snapshot2 > treeSize || snapshot1 < 1 || snapshot2 < 1 || maxBitLen <= 0 {
		return []NodeFetch{}, fmt.Errorf("invalid params s1: %d s2: %d tss: %d, bitlen:%d", snapshot1, snapshot2, treeSize, maxBitLen)
	}

	return snapshotConsistency(snapshot1, snapshot2, treeSize, maxBitLen)
}

// snapshotConsistency does the calculation of consistency proof node addresses between
// two snapshots. Based on the C++ code used by CT but adjusted to fit our situation.
func snapshotConsistency(snapshot1, snapshot2, treeSize int64, maxBitLen int) ([]NodeFetch, error) {
	proof := make([]NodeFetch, 0, bitLen(snapshot2)+1)

	glog.V(vLevel).Infof("snapshotConsistency: %d -> %d", snapshot1, snapshot2)

	level := 0
	node := snapshot1 - 1

	// Compute the (compressed) path to the root of snapshot2.
	// Everything left of 'node' is equal in both trees; no need to record.
	for (node & 1) != 0 {
		glog.V(vvLevel).Infof("Move up: l:%d n:%d", level, node)
		node >>= 1
		level++
	}

	if node != 0 {
		glog.V(vvLevel).Infof("Not root snapshot1: %d", node)
		// Not at the root of snapshot 1, record the node
		n, err := storage.NewNodeIDForTreeCoords(int64(level), node, maxBitLen)
		if err != nil {
			return nil, err
		}
		proof = append(proof, NodeFetch{NodeID: n})
	}

	// Now append the path from this node to the root of snapshot2.
	p, err := pathFromNodeToRootAtSnapshot(node, level, snapshot2, treeSize, maxBitLen)
	if err != nil {
		return nil, err
	}
	return append(proof, p...), nil
}

func pathFromNodeToRootAtSnapshot(node int64, level int, snapshot, treeSize int64, maxBitLen int) ([]NodeFetch, error) {
	glog.V(vLevel).Infof("pathFromNodeToRootAtSnapshot: N:%d, L:%d, S:%d TS:%d", node, level, snapshot, treeSize)
	proof := make([]NodeFetch, 0, bitLen(snapshot)+1)

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
			glog.V(vvLevel).Infof("Not last: S:%d L:%d", sibling, level)
			n, err := storage.NewNodeIDForTreeCoords(int64(level), sibling, maxBitLen)
			if err != nil {
				return nil, err
			}
			proof = append(proof, NodeFetch{NodeID: n})
		} else if sibling == lastNode {
			// The sibling is the last node of the level in the snapshot tree.
			// We might need to recompute a previous hash value here. This can only occur on the
			// rightmost tree nodes because this is the only area of the tree that is not fully populated.
			glog.V(vvLevel).Infof("Last: S:%d L:%d", sibling, level)

			if snapshot == treeSize {
				// No recomputation required as we're using the tree in its current state
				// Account for non existent nodes - these can only be the rightmost node at an
				// intermediate (non leaf) level in the tree so will always be a right sibling.
				n, err := siblingIDSkipLevels(snapshot, lastNode, level, node, maxBitLen)
				if err != nil {
					return nil, err
				}
				proof = append(proof, NodeFetch{NodeID: n})
			} else {
				// We need to recompute this node, as it was at the prior snapshot point. We record
				// the additional fetches needed to do this later
				rehashFetches, err := recomputePastSnapshot(snapshot, treeSize, level, maxBitLen)
				if err != nil {
					return []NodeFetch{}, err
				}

				// Extra check that the recomputation produced one node
				if err = checkRecomputation(rehashFetches); err != nil {
					return []NodeFetch{}, err
				}

				proof = append(proof, rehashFetches...)
			}
		} else {
			glog.V(vvLevel).Infof("Nonexistent: S:%d L:%d", sibling, level)
		}

		// Sibling > lastNode so does not exist, move up
		node >>= 1
		lastNode >>= 1
		level++
	}

	return proof, nil
}

// recomputePastSnapshot does the work to recalculate nodes that need to be rehashed because the
// tree state at the snapshot size differs from the size we've stored it at. The calculations
// also need to take into account missing levels, see the tree diagrams in this file.
// If called with snapshot equal to the tree size returns empty. Otherwise, assuming no errors,
// the output of this should always be exactly one node. Either a copy of one of the nodes in
// the tree or a rehashing of multiple nodes to a single result node.
func recomputePastSnapshot(snapshot, treeSize int64, nodeLevel int, maxBitlen int) ([]NodeFetch, error) {
	glog.V(vLevel).Infof("recompute s:%d ts:%d level:%d", snapshot, treeSize, nodeLevel)

	fetches := []NodeFetch{}

	if snapshot == treeSize {
		// Nothing to do
		return []NodeFetch{}, nil
	} else if snapshot > treeSize {
		return fetches, fmt.Errorf("recomputePastSnapshot: %d does not exist for tree of size %d", snapshot, treeSize)
	}

	// We're recomputing the right hand path, the one to the last leaf
	level := 0
	// This is the index of the last node in the snapshot
	lastNode := snapshot - 1
	// This is the index of the last node that actually exists in the underlying tree
	lastNodeAtLevel := treeSize - 1

	// Work up towards, the root we may find the node we need without needing to rehash if
	// it turns out that the left siblings and parents exist all the way up to the level we're
	// recalculating.
	for (lastNode & 1) != 0 {
		if nodeLevel == level {
			// Then we want a copy of the node at this level
			glog.V(vvLevel).Infof("copying l:%d ln:%d", level, lastNode)
			nodeID, err := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, lastNode^1, maxBitlen)
			if err != nil {
				return []NodeFetch{}, err
			}

			glog.V(vvLevel).Infof("copy node at %s", nodeID.CoordString())
			return append(fetches, NodeFetch{Rehash: false, NodeID: nodeID}), nil
		}

		// Left sibling and parent exist at this snapshot and don't need to be rehashed
		glog.V(vvLevel).Infof("move up ln:%d level:%d", lastNode, level)
		lastNode >>= 1
		lastNodeAtLevel >>= 1
		level++
	}

	glog.V(vvLevel).Infof("done ln:%d level:%d", lastNode, level)

	// lastNode is now the index of a left sibling with no right sibling. This is where the
	// rehashing starts
	savedNodeID, err := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, lastNode^1, maxBitlen)
	glog.V(vvLevel).Infof("root for recompute is: %s", savedNodeID.CoordString())
	if err != nil {
		return []NodeFetch{}, err
	}

	if nodeLevel == level {
		glog.V(vvLevel).Info("emit root (1)")
		return append(fetches, NodeFetch{Rehash: true, NodeID: savedNodeID}), nil
	}

	rehash := false
	rootEmitted := false

	for lastNode != 0 {
		glog.V(vvLevel).Infof("in loop level:%d ln:%d lnal:%d", level, lastNode, lastNodeAtLevel)

		if (lastNode & 1) != 0 {
			nodeID, err := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, (lastNode-1)^1, maxBitlen)
			if err != nil {
				return []NodeFetch{}, err
			}

			if !rehash && !rootEmitted {
				glog.V(vvLevel).Info("emit root (2)")
				fetches = append(fetches, NodeFetch{Rehash: true, NodeID: savedNodeID})
				rootEmitted = true
			}

			glog.V(vvLevel).Infof("rehash with %s", nodeID.CoordString())
			fetches = append(fetches, NodeFetch{Rehash: true, NodeID: nodeID})
			rehash = true
		}

		lastNode >>= 1
		lastNodeAtLevel >>= 1
		level++

		if nodeLevel == level && !rootEmitted {
			glog.V(vvLevel).Info("emit root (3)")
			return append(fetches, NodeFetch{Rehash: rehash, NodeID: savedNodeID}), nil
		}

		// Exit early if we've gone far enough up the tree to hit the level we're recomputing
		if level == nodeLevel {
			glog.V(vvLevel).Infof("returning fetches early: %v", fetches)
			return fetches, nil
		}
	}

	glog.V(vvLevel).Infof("returning fetches: %v", fetches)
	return fetches, nil
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
		glog.V(vvLevel).Infof("Move down: S:%d L:%d LN:%d", sibling, level, lastNode)
	}

	return level, sibling
}

// checkRecomputation carries out an additional check that the results of recomputePastSnapshot
// are valid. There must be at least one fetch. All fetches must have the same rehash state and if
// there is only one fetch then it must not be a rehash. If all checks pass then the fetches
// represent one node after rehashing is completed.
func checkRecomputation(fetches []NodeFetch) error {
	if len(fetches) == 0 {
		return errors.New("recomputePastSnapshot returned nothing")
	} else if len(fetches) == 1 {
		if fetches[0].Rehash {
			return errors.New("recomputePastSnapshot returned invalid rehash")
		}
	} else {
		for i := range fetches {
			if i > 0 && fetches[i].Rehash != fetches[0].Rehash {
				return errors.New("recomputePastSnapshot returned multiple nodes")
			}
		}
	}

	return nil
}

// siblingIDSkipLevels creates a new NodeID for the supplied node, accounting for levels skipped
// in storage. Note that it returns an ID for the node sibling so care should be taken to pass the
// correct value for the node parameter.
func siblingIDSkipLevels(snapshot, lastNode int64, level int, node int64, maxBitLen int) (storage.NodeID, error) {
	l, sibling := skipMissingLevels(snapshot, lastNode, level, node)
	return storage.NewNodeIDForTreeCoords(int64(l), sibling, maxBitLen)
}

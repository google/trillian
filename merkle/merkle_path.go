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
	"errors"
	"fmt"
	"math/bits"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/compact"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Verbosity levels for logging of debug related items
const vLevel = 2
const vvLevel = 4

// NodeFetch bundles a nodeID with additional information on how to use the node to construct the
// correct proof.
type NodeFetch struct {
	ID     compact.NodeID
	Rehash bool
}

// checkSnapshot performs a couple of simple sanity checks on ss and treeSize
// and returns an error if there's a problem.
func checkSnapshot(ssDesc string, ss, treeSize int64) error {
	if ss < 1 {
		return fmt.Errorf("%s %d < 1", ssDesc, ss)
	}
	if ss > treeSize {
		return fmt.Errorf("%s %d > treeSize %d", ssDesc, ss, treeSize)
	}
	return nil
}

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to build an
// inclusion proof for a specified leaf and tree size. The snapshot parameter
// is the tree size being queried for, treeSize is the actual size of the tree
// at the revision we are using to fetch nodes (this can be > snapshot).
func CalcInclusionProofNodeAddresses(snapshot, index, treeSize int64) ([]NodeFetch, error) {
	if err := checkSnapshot("snapshot", snapshot, treeSize); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: %v", err)
	}
	if index >= snapshot {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: index %d is >= snapshot %d", index, snapshot)
	}
	if index < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: index %d is < 0", index)
	}

	return pathFromNodeToRootAtSnapshot(index, 0, snapshot, treeSize)
}

// CalcConsistencyProofNodeAddresses returns the tree node IDs needed to build
// a consistency proof between two specified tree sizes. snapshot1 and
// snapshot2 represent the two tree sizes for which consistency should be
// proved, treeSize is the actual size of the tree at the revision we are using
// to fetch nodes (this can be > snapshot2).
//
// The caller is responsible for checking that the input tree sizes correspond
// to valid tree heads. All returned NodeIDs are tree coordinates within the
// new tree. It is assumed that they will be fetched from storage at a revision
// corresponding to the STH associated with the treeSize parameter.
func CalcConsistencyProofNodeAddresses(snapshot1, snapshot2, treeSize int64) ([]NodeFetch, error) {
	if err := checkSnapshot("snapshot1", snapshot1, treeSize); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: %v", err)
	}
	if err := checkSnapshot("snapshot2", snapshot2, treeSize); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: %v", err)
	}
	if snapshot1 > snapshot2 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: snapshot1 %d > snapshot2 %d", snapshot1, snapshot2)
	}

	return snapshotConsistency(snapshot1, snapshot2, treeSize)
}

// snapshotConsistency does the calculation of consistency proof node addresses between
// two snapshots. Based on the C++ code used by CT but adjusted to fit our situation.
func snapshotConsistency(snapshot1, snapshot2, treeSize int64) ([]NodeFetch, error) {
	proof := make([]NodeFetch, 0, bits.Len64(uint64(snapshot2))+1)

	glog.V(vLevel).Infof("snapshotConsistency: %d -> %d", snapshot1, snapshot2)

	if snapshot1 == snapshot2 {
		return proof, nil
	}

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
		n := compact.NewNodeID(uint(level), uint64(node))
		proof = append(proof, NodeFetch{ID: n})
	}

	// Now append the path from this node to the root of snapshot2.
	p, err := pathFromNodeToRootAtSnapshot(node, level, snapshot2, treeSize)
	if err != nil {
		return nil, err
	}
	return append(proof, p...), nil
}

func pathFromNodeToRootAtSnapshot(node int64, level int, snapshot, treeSize int64) ([]NodeFetch, error) {
	glog.V(vLevel).Infof("pathFromNodeToRootAtSnapshot(%d, %d, %d, %d)", node, level, snapshot, treeSize)
	proof := make([]NodeFetch, 0, bits.Len64(uint64(snapshot))+1)

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
			if glog.V(vvLevel) {
				glog.Infof("Not last: S:%d L:%d", sibling, level)
			}
			n := compact.NewNodeID(uint(level), uint64(sibling))
			proof = append(proof, NodeFetch{ID: n})
		} else if sibling == lastNode {
			// The sibling is the last node of the level in the snapshot tree.
			// We might need to recompute a previous hash value here. This can only occur on the
			// rightmost tree nodes because this is the only area of the tree that is not fully populated.
			if glog.V(vvLevel) {
				glog.Infof("Last: S:%d L:%d", sibling, level)
			}

			if snapshot == treeSize {
				// No recomputation required as we're using the tree in its current state
				// Account for non existent nodes - these can only be the rightmost node at an
				// intermediate (non leaf) level in the tree so will always be a right sibling.
				n := siblingIDSkipLevels(snapshot, lastNode, level, node)
				proof = append(proof, NodeFetch{ID: n})
			} else {
				// We need to recompute this node, as it was at the prior snapshot point. We record
				// the additional fetches needed to do this later
				rehashFetches, err := recomputePastSnapshot(snapshot, treeSize, level)
				if err != nil {
					return nil, err
				}

				// Extra check that the recomputation produced one node
				if err = checkRecomputation(rehashFetches); err != nil {
					return nil, err
				}

				proof = append(proof, rehashFetches...)
			}
		} else {
			if glog.V(vvLevel) {
				glog.Infof("Nonexistent: S:%d L:%d", sibling, level)
			}
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
// the output of this should always be exactly one node after resolving any rehashing.
// Either a copy of one of the nodes in the tree or a rehashing of multiple nodes to a single
// result node with the value it would have had if the prior snapshot had been stored.
func recomputePastSnapshot(snapshot, treeSize int64, nodeLevel int) ([]NodeFetch, error) {
	glog.V(vLevel).Infof("recompute s:%d ts:%d level:%d", snapshot, treeSize, nodeLevel)

	fetches := []NodeFetch{}

	if snapshot == treeSize {
		// Nothing to do
		return nil, nil
	} else if snapshot > treeSize {
		return nil, fmt.Errorf("recomputePastSnapshot: %d does not exist for tree of size %d", snapshot, treeSize)
	}

	// We're recomputing the right hand path, the one to the last leaf
	level := 0
	// This is the index of the last node in the snapshot
	lastNode := snapshot - 1
	// This is the index of the last node that actually exists in the underlying tree
	lastNodeAtLevel := treeSize - 1

	// Work up towards the root. We may find the node we need without needing to rehash if
	// it turns out that the tree is complete up to the level we're recalculating at this
	// snapshot.
	for (lastNode & 1) != 0 {
		if nodeLevel == level {
			// Then we want a copy of the node at this level
			if glog.V(vvLevel) {
				glog.Infof("copying l:%d ln:%d", level, lastNode)
			}
			nodeID := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, lastNode^1)
			return append(fetches, NodeFetch{Rehash: false, ID: nodeID}), nil
		}

		// Left sibling and parent exist at this snapshot and don't need to be rehashed
		if glog.V(vvLevel) {
			glog.Infof("move up ln:%d level:%d", lastNode, level)
		}
		lastNode >>= 1
		lastNodeAtLevel >>= 1
		level++
	}

	if glog.V(vvLevel) {
		glog.Infof("done ln:%d level:%d", lastNode, level)
	}

	// lastNode is now the index of a left sibling with no right sibling. This is where the
	// rehashing starts
	savedNodeID := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, lastNode^1)

	if nodeLevel == level {
		return append(fetches, NodeFetch{Rehash: true, ID: savedNodeID}), nil
	}

	rehash := false
	subRootEmitted := false // whether we've added the recomputed subtree root to the path yet

	// Move towards the tree root (increasing level). Exit when we reach the root or the
	// level that is being recomputed. Defer emitting the subtree root to the path until
	// the appropriate point because we don't immediately know whether it's part of the
	// rehashing.
	for lastNode != 0 {
		if glog.V(vvLevel) {
			glog.Infof("in loop level:%d ln:%d lnal:%d", level, lastNode, lastNodeAtLevel)
		}

		if (lastNode & 1) != 0 {
			nodeID := siblingIDSkipLevels(snapshot, lastNodeAtLevel, level, (lastNode-1)^1)
			if !rehash && !subRootEmitted {
				fetches = append(fetches, NodeFetch{Rehash: true, ID: savedNodeID})
				subRootEmitted = true
			}

			if glog.V(vvLevel) {
				glog.Infof("rehash with %+v", nodeID)
			}
			fetches = append(fetches, NodeFetch{Rehash: true, ID: nodeID})
			rehash = true
		}

		lastNode >>= 1
		lastNodeAtLevel >>= 1
		level++

		if nodeLevel == level && !subRootEmitted {
			return append(fetches, NodeFetch{Rehash: rehash, ID: savedNodeID}), nil
		}

		// Exit early if we've gone far enough up the tree to hit the level we're recomputing
		if level == nodeLevel {
			if glog.V(vvLevel) {
				glog.Infof("returning fetches early: %v", fetches)
			}
			return fetches, nil
		}
	}

	if glog.V(vvLevel) {
		glog.Infof("returning fetches: %v", fetches)
	}
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
	b := uint64(ts - 1)
	// Test the bit in the path for the requested level
	mask := uint64(1) << uint64(level-1)

	return b&mask != 0
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
		if glog.V(vvLevel) {
			glog.Infof("Move down: S:%d L:%d LN:%d", sibling, level, lastNode)
		}
	}

	return level, sibling
}

// checkRecomputation carries out an additional check that the results of recomputePastSnapshot
// are valid. There must be at least one fetch. All fetches must have the same rehash state and if
// there is only one fetch then it must not be a rehash. If all checks pass then the fetches
// represent one node after rehashing is completed.
func checkRecomputation(fetches []NodeFetch) error {
	switch len(fetches) {
	case 0:
		return errors.New("recomputePastSnapshot returned nothing")
	case 1:
		if fetches[0].Rehash {
			return fmt.Errorf("recomputePastSnapshot returned invalid rehash: %v", fetches)
		}
	default:
		for i := range fetches {
			if i > 0 && fetches[i].Rehash != fetches[0].Rehash {
				return fmt.Errorf("recomputePastSnapshot returned mismatched rehash nodes: %v", fetches)
			}
		}
	}

	return nil
}

// siblingIDSkipLevels creates a new NodeID for the supplied node, accounting for levels skipped
// in storage. Note that it returns an ID for the node sibling so care should be taken to pass the
// correct value for the node parameter.
func siblingIDSkipLevels(snapshot, lastNode int64, level int, node int64) compact.NodeID {
	l, sibling := skipMissingLevels(snapshot, lastNode, level, node)
	return compact.NewNodeID(uint(l), uint64(sibling))
}

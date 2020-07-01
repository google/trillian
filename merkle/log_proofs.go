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

package merkle

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/google/trillian/merkle/compact"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
//
// Use Rehash function to compose the proof after the node hashes are fetched.
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
	// Note: If snapshot < treeSize, the storage might not contain the
	// "ephemeral" node of this proof, so rehashing is needed.
	return proofNodes(uint64(index), 0, uint64(snapshot), snapshot < treeSize), nil
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
//
// Use Rehash function to compose the proof after the node hashes are fetched.
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

// snapshotConsistency does the calculation of consistency proof node addresses
// between two snapshots in a bigger tree of the given size.
func snapshotConsistency(snapshot1, snapshot2, treeSize int64) ([]NodeFetch, error) {
	if snapshot1 == snapshot2 {
		return []NodeFetch{}, nil
	}

	// TODO(pavelkalinnikov): Make the capacity estimate accurate.
	proof := make([]NodeFetch, 0, bits.Len64(uint64(snapshot2))+1)

	// Find the biggest perfect subtree that ends at snapshot1.
	level := uint(bits.TrailingZeros64(uint64(snapshot1)))
	index := uint64((snapshot1 - 1)) >> level
	// If it does not cover the whole snapshot1 tree, add this node to the proof.
	if index != 0 {
		n := compact.NewNodeID(level, index)
		proof = append(proof, NodeFetch{ID: n})
	}

	// Now append the path from this node to the root of snapshot2.
	p := proofNodes(index, level, uint64(snapshot2), snapshot2 < treeSize)
	return append(proof, p...), nil
}

// proofNodes returns the node IDs necessary to prove that the (level, index)
// node is included in the Merkle tree of the given size.
func proofNodes(index uint64, level uint, size uint64, rehash bool) []NodeFetch {
	// [begin, end) is the leaves range covered by the (level, index) node.
	begin, end := index<<level, (index+1)<<level
	// To prove inclusion of range [begin, end), we only need nodes of compact
	// range [0, begin) and [end, size). Further down, we need the nodes ordered
	// by level from leaves towards the root.
	left := reverse(compact.RangeNodes(0, begin))
	// We decompose the [end, size) range into [end, end+l) and [end+l, size).
	// The first one (named `middle` here) contains all the nodes that don't have
	// a left sibling within [end, size), and the second one (named `right`
	// below) contains all the nodes that don't have a right sibling.
	l, r := compact.Decompose(end, size)
	middle := compact.RangeNodes(end, end+l)

	// Nodes that don't have a right sibling (i.e. the right border of the tree)
	// are special, because their hashes are collapsed into a single "ephemeral"
	// hash. This hash is already known if rehash==false, otherwise the caller
	// needs to compute it based on the hashes of compact range [end+l, size).
	var right []compact.NodeID
	if r != 0 {
		if rehash {
			right = reverse(compact.RangeNodes(end+l, size))
			rehash = len(right) > 1
		} else {
			// The parent of the highest node in [end+l, size) is "ephemeral".
			lvl := uint(bits.Len64(r))
			// Except when [end+l, size) is a perfect subtree, in which case we just
			// take the root node.
			if r&(r-1) == 0 {
				lvl--
			}
			right = []compact.NodeID{compact.NewNodeID(lvl, (end+l)>>lvl)}
		}
	}

	// The level in the ordered list of nodes where the rehashed nodes appear in
	// lieu of the "ephemeral" node. This is equal to the level where the path to
	// the `begin` index diverges from the path to `size`.
	rehashLevel := uint(bits.Len64(begin^size) - 1)

	// Merge the three compact ranges into a single proof ordered by node level
	// from leaves towards the root, i.e. the format specified in RFC 6962.
	proof := make([]NodeFetch, 0, len(left)+len(middle)+len(right))
	i, j := 0, 0
	for l, levels := level, uint(bits.Len64(size-1)); l < levels; l++ {
		if i < len(left) && left[i].Level == l {
			proof = append(proof, NodeFetch{ID: left[i]})
			i++
		} else if j < len(middle) && middle[j].Level == l {
			proof = append(proof, NodeFetch{ID: middle[j]})
			j++
		}
		if l == rehashLevel {
			for _, id := range right {
				proof = append(proof, NodeFetch{ID: id, Rehash: rehash})
			}
		}
	}

	return proof
}

// Rehash computes the proof based on the slice of NodeFetch structs, and the
// corresponding hashes of these nodes. The slices must be of the same length.
// The hc parameter computes node's hash based on hashes of its children.
//
// Warning: The passed-in slice of hashes can be modified in-place.
func Rehash(h [][]byte, nf []NodeFetch, hc func(left, right []byte) []byte) ([][]byte, error) {
	if len(h) != len(nf) {
		return nil, errors.New("slice lengths mismatch")
	}
	cursor := 0
	for i, ln := 0, len(h); i < ln; i, cursor = i+1, cursor+1 {
		hash := h[i]
		if nf[i].Rehash {
			for i++; i < len(nf) && nf[i].Rehash; i++ {
				hash = hc(h[i], hash)
			}
			i--
		}
		h[cursor] = hash
	}
	return h[:cursor], nil
}

func reverse(ids []compact.NodeID) []compact.NodeID {
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

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

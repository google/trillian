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

package server

import (
	"fmt"

	"github.com/google/trillian"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func validateGetInclusionProofRequest(req *trillian.GetInclusionProofRequest) error {
	if req.TreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofRequest.TreeSize: %v, want > 0", req.TreeSize)
	}
	if req.LeafIndex < 0 {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofRequest.LeafIndex: %v, want >= 0", req.LeafIndex)
	}
	if req.LeafIndex >= req.TreeSize {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofRequest.LeafIndex: %v >= TreeSize: %v, want < ", req.LeafIndex, req.TreeSize)
	}
	return nil
}

func validateGetInclusionProofByHashRequest(req *trillian.GetInclusionProofByHashRequest) error {
	if req.TreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofByHashRequest.TreeSize: %v, want > 0", req.TreeSize)
	}
	if err := validateLeafHash(req.LeafHash); err != nil {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofByHashRequest.LeafHash: %v", err)
	}
	return nil
}

func validateGetLeavesByHashRequest(req *trillian.GetLeavesByHashRequest) error {
	if len(req.LeafHash) == 0 {
		return status.Error(codes.InvalidArgument, "GetLeavesByHashRequest.LeafHash empty")
	}
	for i, hash := range req.LeafHash {
		if err := validateLeafHash(hash); err != nil {
			return status.Errorf(codes.InvalidArgument, "GetLeavesByHashRequest.LeafHash[%v]: %v", i, err)
		}
	}
	return nil
}

func validateGetLeavesByIndexRequest(req *trillian.GetLeavesByIndexRequest) error {
	if len(req.LeafIndex) == 0 {
		return status.Error(codes.InvalidArgument, "GetLeavesByIndexRequest.LeafIndex empty")
	}
	for i, leafIndex := range req.LeafIndex {
		if leafIndex < 0 {
			return status.Errorf(codes.InvalidArgument, "GetLeavesByIndexRequest.LeafIndex[%v]: %v, want >= 0", i, leafIndex)
		}
	}
	return nil
}

func validateGetLeavesByRangeRequest(req *trillian.GetLeavesByRangeRequest) error {
	if req.StartIndex < 0 {
		return status.Errorf(codes.InvalidArgument, "GetLeavesByRangeRequest.StartIndex: %v, want >= 0", req.StartIndex)
	}
	if req.Count <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetLeavesByRangeRequest.Count: %v, want > 0", req.Count)
	}
	return nil
}

func validateGetConsistencyProofRequest(req *trillian.GetConsistencyProofRequest) error {
	if req.FirstTreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetConsistencyProofRequest.FirstTreeSize: %v, want > 0", req.FirstTreeSize)
	}
	if req.SecondTreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetConsistencyProofRequest.SecondTreeSize: %v, want > 0", req.SecondTreeSize)
	}
	if req.SecondTreeSize < req.FirstTreeSize {
		return status.Errorf(codes.InvalidArgument, "GetConsistencyProofRequest.FirstTreeSize: %v < GetConsistencyProofRequest.SecondTreeSize: %v, want >= ", req.FirstTreeSize, req.SecondTreeSize)
	}
	return nil
}

func validateGetEntryAndProofRequest(req *trillian.GetEntryAndProofRequest) error {
	if req.TreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetEntryAndProofRequest.TreeSize: %v, want > 0", req.TreeSize)
	}
	if req.LeafIndex < 0 {
		return status.Errorf(codes.InvalidArgument, "GetEntryAndProofRequest.LeafIndex: %v, want >= 0", req.LeafIndex)
	}
	if req.LeafIndex >= req.TreeSize {
		return status.Errorf(codes.InvalidArgument, "GetEntryAndProofRequest.LeafIndex: %v >= TreeSize: %v, want < ", req.LeafIndex, req.TreeSize)
	}
	return nil
}

func validateQueueLeafRequest(req *trillian.QueueLeafRequest) error {
	if err := validateLogLeaf(req.Leaf); err != nil {
		// validateLogLeaf errors chain nicely with "Leaf".
		return status.Errorf(codes.InvalidArgument, "QueueLeafRequest.Leaf%v", err)
	}
	return nil
}

func validateAddSequencedLeafRequest(req *trillian.AddSequencedLeafRequest) error {
	if err := validateLogLeaf(req.Leaf); err != nil {
		// validateLogLeaf errors chain nicely with "Leaf".
		return status.Errorf(codes.InvalidArgument, "AddSequencedLeafRequest.Leaf%v", err)
	}
	return nil
}

func validateQueueLeavesRequest(req *trillian.QueueLeavesRequest) error {
	if err := validateLogLeaves(req.Leaves); err != nil {
		// validateLogLeaves errors chain nicely with "QueueLeavesRequest.".
		return status.Errorf(codes.InvalidArgument, "QueueLeavesRequest.%v", err)
	}
	return nil
}

func validateAddSequencedLeavesRequest(req *trillian.AddSequencedLeavesRequest) error {
	if err := validateLogLeaves(req.Leaves); err != nil {
		// validateLogLeaves errors chain nicely with "AddSequencedLeavesRequest.".
		return status.Errorf(codes.InvalidArgument, "AddSequencedLeavesRequest.%v", err)
	}
	lastIndex := int64(-1)
	for i, leaf := range req.Leaves {
		if leaf.LeafIndex <= lastIndex {
			return status.Errorf(codes.OutOfRange, "AddSequencedLeavesRequest.Leaves[%v..%v].LeafIndex not sorted", i-1, i)
		}
		lastIndex = leaf.LeafIndex
	}
	return nil
}

func validateLeafHash(hash []byte) error {
	if len(hash) == 0 {
		return fmt.Errorf("leaf hash empty")
	}
	return nil
}

func validateLogLeaves(leaves []*trillian.LogLeaf) error {
	if len(leaves) == 0 {
		return status.Error(codes.InvalidArgument, "Leaves empty")
	}
	for i, leaf := range leaves {
		if err := validateLogLeaf(leaf); err != nil {
			// validateLogLeaf errors are meant to chain nicely with "Leaves".
			return status.Errorf(codes.InvalidArgument, "Leaves[%v]%v", i, err)
		}
	}
	return nil
}

// validateLogLeaf validates a leaf and returns an error in the format
// ".$Field: $message" or " $message", so it chains nicely with a path-like
// string to the leaf field (e.g.: "req.Leaf%v").
func validateLogLeaf(leaf *trillian.LogLeaf) error {
	if leaf == nil {
		return status.Errorf(codes.InvalidArgument, " empty")
	}
	switch {
	case len(leaf.LeafValue) == 0:
		return status.Error(codes.InvalidArgument, ".LeafValue: empty")
	case leaf.LeafIndex < 0:
		return status.Errorf(codes.InvalidArgument, ".LeafIndex: %v, want >= 0", leaf.LeafIndex)
	}
	return nil
}

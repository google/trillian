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

package server

import (
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
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

func validateGetInclusionProofByHashRequest(req *trillian.GetInclusionProofByHashRequest, hasher hashers.LogHasher) error {
	if req.TreeSize <= 0 {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofByHashRequest.TreeSize: %v, want > 0", req.TreeSize)
	}
	if err := validateLeafHash(req.LeafHash, hasher); err != nil {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofByHashRequest.LeafHash: %v", err)
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
		return status.Errorf(codes.InvalidArgument, "GetConsistencyProofRequest.SecondTreeSize: %v < GetConsistencyProofRequest.FirstTreeSize: %v, want >= ", req.SecondTreeSize, req.FirstTreeSize)
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

func validateAddSequencedLeavesRequest(req *trillian.AddSequencedLeavesRequest) error {
	prefix := "AddSequencedLeavesRequest"
	if err := validateLogLeaves(req.Leaves, prefix); err != nil {
		return err
	}

	// Note: Not empty, as verified by validateLogLeaves.
	nextIndex := req.Leaves[0].LeafIndex
	for i, leaf := range req.Leaves {
		if leaf.LeafIndex != nextIndex {
			return status.Errorf(codes.FailedPrecondition, "%v.Leaves[%v].LeafIndex=%v, want %v", prefix, i, leaf.LeafIndex, nextIndex)
		}
		nextIndex++
	}
	return nil
}

func validateLogLeaves(leaves []*trillian.LogLeaf, errPrefix string) error {
	if len(leaves) == 0 {
		return status.Errorf(codes.InvalidArgument, "%v.Leaves empty", errPrefix)
	}
	for i, leaf := range leaves {
		if err := validateLogLeaf(leaf, ""); err != nil {
			return status.Errorf(codes.InvalidArgument, "%v.Leaves[%v]%v", errPrefix, i, err)
		}
	}
	return nil
}

func validateLogLeaf(leaf *trillian.LogLeaf, errPrefix string) error {
	if leaf == nil {
		return status.Errorf(codes.InvalidArgument, "%v empty", errPrefix)
	}
	switch {
	case len(leaf.LeafValue) == 0:
		return status.Errorf(codes.InvalidArgument, "%v.LeafValue: empty", errPrefix)
	case leaf.LeafIndex < 0:
		return status.Errorf(codes.InvalidArgument, "%v.LeafIndex: %v, want >= 0", errPrefix, leaf.LeafIndex)
	}
	return nil
}

func validateLeafHash(hash []byte, hasher hashers.LogHasher) error {
	if got, want := len(hash), hasher.Size(); got != want {
		return fmt.Errorf("%d bytes, want %d", got, want)
	}
	return nil
}

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
	if len(req.LeafHash) == 0 {
		return status.Errorf(codes.InvalidArgument, "GetInclusionProofByHashRequest.LeafHash empty")
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

func validateQueueLeavesRequest(req *trillian.QueueLeavesRequest) error {
	if len(req.Leaves) == 0 {
		return status.Errorf(codes.InvalidArgument, "len(QueueLeavesRequest.Leaves)=0, want > 0")
	}
	return nil
}

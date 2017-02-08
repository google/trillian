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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func validateGetInclusionProofRequest(req *trillian.GetInclusionProofRequest) error {
	if req.TreeSize <= 0 {
		return grpc.Errorf(codes.InvalidArgument, "TreeSize: %v, want > 0", req.TreeSize)
	}
	if req.LeafIndex < 0 {
		return grpc.Errorf(codes.InvalidArgument, "LeafIndex: %v, want >= 0", req.LeafIndex)
	}
	if req.LeafIndex >= req.TreeSize {
		return grpc.Errorf(codes.InvalidArgument, "LeafIndex: %v >= TreeSize: %v, want < ", req.LeafIndex, req.TreeSize)
	}
	return nil
}

func validateGetInclusionProofByHashRequest(req *trillian.GetInclusionProofByHashRequest) error {
	if req.TreeSize <= 0 {
		return grpc.Errorf(codes.InvalidArgument, "TreeSize: %v, want > 0", req.TreeSize)
	}
	if len(req.LeafHash) == 0 {
		return grpc.Errorf(codes.InvalidArgument, "Empty Leafhash: %v", req.LeafHash)
	}
	return nil
}

func validateGetConsistencyProofRequest(req *trillian.GetConsistencyProofRequest) error {
	if req.FirstTreeSize <= 0 {
		return grpc.Errorf(codes.InvalidArgument, "FirstTreeSize: %v, want > 0", req.FirstTreeSize)
	}
	if req.SecondTreeSize <= 0 {
		return grpc.Errorf(codes.InvalidArgument, "SecondTreeSize: %v, want > 0", req.SecondTreeSize)
	}
	if req.SecondTreeSize <= req.FirstTreeSize {
		return grpc.Errorf(codes.InvalidArgument, "FirstTreeSize: %v < SecondTreeSize: %v, want > ", req.FirstTreeSize, req.SecondTreeSize)
	}
	return nil
}

func validateGetEntryAndProofRequest(req *trillian.GetEntryAndProofRequest) error {
	if req.TreeSize <= 0 {
		return grpc.Errorf(codes.InvalidArgument, "TreeSize: %v, want > 0", req.TreeSize)
	}
	if req.LeafIndex < 0 {
		return grpc.Errorf(codes.InvalidArgument, "LeafIndex: %v, want >= 0", req.LeafIndex)
	}
	if req.LeafIndex >= req.TreeSize {
		return grpc.Errorf(codes.InvalidArgument, "LeafIndex: %v >= TreeSize: %v, want < ", req.LeafIndex, req.TreeSize)
	}
	return nil
}

func validateQueueLeavesRequest(req *trillian.QueueLeavesRequest) error {
	if len(req.Leaves) == 0 {
		return grpc.Errorf(codes.InvalidArgument, "len(leaves)=0, want > 0")
	}
	return nil
}

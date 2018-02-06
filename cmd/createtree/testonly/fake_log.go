// Copyright 2018 Google Inc. All Rights Reserved.
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

package testonly

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tpb "github.com/google/trillian"
)

// LogServer only stores tree size.
type LogServer struct {
	TreeSize int64
	// InitErr will be returned by InitLog if not nil.
	InitErr error
}

// NewTrillianLogServer returns a fake trillian log server.
func NewTrillianLogServer() *LogServer {
	return &LogServer{
		TreeSize: -1,
	}
}

// QueueLeaf increments the size of the tree.
func (l *LogServer) QueueLeaf(context.Context, *tpb.QueueLeafRequest) (*tpb.QueueLeafResponse, error) {
	l.TreeSize++
	return nil, nil
}

// QueueLeaves is not implemented.
func (*LogServer) QueueLeaves(context.Context, *tpb.QueueLeavesRequest) (*tpb.QueueLeavesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetInclusionProof returns an empty proof.
func (*LogServer) GetInclusionProof(context.Context, *tpb.GetInclusionProofRequest) (*tpb.GetInclusionProofResponse, error) {
	return &tpb.GetInclusionProofResponse{}, nil
}

// GetInclusionProofByHash is not implemented.
func (*LogServer) GetInclusionProofByHash(context.Context, *tpb.GetInclusionProofByHashRequest) (*tpb.GetInclusionProofByHashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetConsistencyProof returns an empty proof.
func (*LogServer) GetConsistencyProof(context.Context, *tpb.GetConsistencyProofRequest) (*tpb.GetConsistencyProofResponse, error) {
	return &tpb.GetConsistencyProofResponse{}, nil
}

// GetLatestSignedLogRoot returns the current tree size.
func (l *LogServer) GetLatestSignedLogRoot(context.Context, *tpb.GetLatestSignedLogRootRequest) (*tpb.GetLatestSignedLogRootResponse, error) {
	return &tpb.GetLatestSignedLogRootResponse{
		SignedLogRoot: &tpb.SignedLogRoot{
			TreeSize: l.TreeSize,
		},
	}, nil
}

// GetSequencedLeafCount is not implemented.
func (*LogServer) GetSequencedLeafCount(context.Context, *tpb.GetSequencedLeafCountRequest) (*tpb.GetSequencedLeafCountResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetLeavesByIndex is not implemented.
func (*LogServer) GetLeavesByIndex(context.Context, *tpb.GetLeavesByIndexRequest) (*tpb.GetLeavesByIndexResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetLeavesByHash is not implemented.
func (*LogServer) GetLeavesByHash(context.Context, *tpb.GetLeavesByHashRequest) (*tpb.GetLeavesByHashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetEntryAndProof is not implemented.
func (*LogServer) GetEntryAndProof(context.Context, *tpb.GetEntryAndProofRequest) (*tpb.GetEntryAndProofResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// GetLeavesByRange is not implemented.
func (*LogServer) GetLeavesByRange(context.Context, *tpb.GetLeavesByRangeRequest) (*tpb.GetLeavesByRangeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "")
}

// InitLog returns an error if s.InitErr is set, and an empty InitLogResponse
// struct otherwise.
func (l *LogServer) InitLog(ctx context.Context, in *tpb.InitLogRequest) (*tpb.InitLogResponse, error) {
	if l.InitErr != nil {
		return nil, l.InitErr
	}
	l.TreeSize = 0
	return &tpb.InitLogResponse{}, nil
}

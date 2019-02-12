// Copyright 2017 Google Inc. All Rights Reserved.
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

package client

import (
	"context"
	"math/rand"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"google.golang.org/grpc"
)

// MutatingLogClient supports applying mutations to the return values of the TrillianLogClient
// for testing.
type MutatingLogClient struct {
	c                      trillian.TrillianLogClient
	mutateInclusionProof   bool
	mutateConsistencyProof bool
}

// QueueLeaf forwards requests.
func (c *MutatingLogClient) QueueLeaf(ctx context.Context, in *trillian.QueueLeafRequest, opts ...grpc.CallOption) (*trillian.QueueLeafResponse, error) {
	return c.c.QueueLeaf(ctx, in)
}

// QueueLeaves forwards requests.
func (c *MutatingLogClient) QueueLeaves(ctx context.Context, in *trillian.QueueLeavesRequest, opts ...grpc.CallOption) (*trillian.QueueLeavesResponse, error) {
	return c.c.QueueLeaves(ctx, in)
}

// AddSequencedLeaf forwards requests.
func (c *MutatingLogClient) AddSequencedLeaf(ctx context.Context, in *trillian.AddSequencedLeafRequest, opts ...grpc.CallOption) (*trillian.AddSequencedLeafResponse, error) {
	return c.c.AddSequencedLeaf(ctx, in)
}

// AddSequencedLeaves forwards requests.
func (c *MutatingLogClient) AddSequencedLeaves(ctx context.Context, in *trillian.AddSequencedLeavesRequest, opts ...grpc.CallOption) (*trillian.AddSequencedLeavesResponse, error) {
	return c.c.AddSequencedLeaves(ctx, in)
}

// GetInclusionProof forwards requests and optionally corrupts the response.
func (c *MutatingLogClient) GetInclusionProof(ctx context.Context, in *trillian.GetInclusionProofRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofResponse, error) {
	resp, err := c.c.GetInclusionProof(ctx, in)
	if c.mutateInclusionProof {
		i := rand.Intn(len(resp.Proof.Hashes))
		j := rand.Intn(len(resp.Proof.Hashes[i]))
		resp.Proof.Hashes[i][j] ^= 4
	}
	return resp, err
}

// GetInclusionProofByHash forwards requests and optionaly corrupts responses.
func (c *MutatingLogClient) GetInclusionProofByHash(ctx context.Context, in *trillian.GetInclusionProofByHashRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofByHashResponse, error) {
	resp, err := c.c.GetInclusionProofByHash(ctx, in)
	if err != nil {
		return nil, err
	}
	if c.mutateInclusionProof {
		h := rand.Intn(len(resp.Proof))
		if len(resp.Proof[h].Hashes) == 0 {
			glog.Warningf("Inclusion proof not modified because treesize = 0")
			return resp, nil
		}
		i := rand.Intn(len(resp.Proof[h].Hashes))
		j := rand.Intn(len(resp.Proof[h].Hashes[i]))
		resp.Proof[h].Hashes[i][j] ^= 4
	}
	return resp, nil
}

// GetConsistencyProof forwards requests and optionally corrupts responses.
func (c *MutatingLogClient) GetConsistencyProof(ctx context.Context, in *trillian.GetConsistencyProofRequest, opts ...grpc.CallOption) (*trillian.GetConsistencyProofResponse, error) {
	resp, err := c.c.GetConsistencyProof(ctx, in)
	if err != nil {
		return nil, err
	}
	if c.mutateConsistencyProof {
		if len(resp.Proof.Hashes) == 0 {
			glog.Warningf("Consistency proof not modified because len(Hashes) = 0")
			return resp, nil
		}
		i := rand.Intn(len(resp.Proof.Hashes))
		j := rand.Intn(len(resp.Proof.Hashes[i]))
		resp.Proof.Hashes[i][j] ^= 4
	}
	return resp, nil
}

// GetLatestSignedLogRoot forwards requests.
func (c *MutatingLogClient) GetLatestSignedLogRoot(ctx context.Context, in *trillian.GetLatestSignedLogRootRequest, opts ...grpc.CallOption) (*trillian.GetLatestSignedLogRootResponse, error) {
	return c.c.GetLatestSignedLogRoot(ctx, in)
}

// GetSequencedLeafCount forwards requests.
func (c *MutatingLogClient) GetSequencedLeafCount(ctx context.Context, in *trillian.GetSequencedLeafCountRequest, opts ...grpc.CallOption) (*trillian.GetSequencedLeafCountResponse, error) {
	return c.c.GetSequencedLeafCount(ctx, in)
}

// GetLeavesByIndex forwards requests.
func (c *MutatingLogClient) GetLeavesByIndex(ctx context.Context, in *trillian.GetLeavesByIndexRequest, opts ...grpc.CallOption) (*trillian.GetLeavesByIndexResponse, error) {
	return c.c.GetLeavesByIndex(ctx, in)
}

// GetLeavesByRange forwards requests.
func (c *MutatingLogClient) GetLeavesByRange(ctx context.Context, in *trillian.GetLeavesByRangeRequest, opts ...grpc.CallOption) (*trillian.GetLeavesByRangeResponse, error) {
	return c.c.GetLeavesByRange(ctx, in)
}

// GetLeavesByHash forwards requests.
func (c *MutatingLogClient) GetLeavesByHash(ctx context.Context, in *trillian.GetLeavesByHashRequest, opts ...grpc.CallOption) (*trillian.GetLeavesByHashResponse, error) {
	return c.c.GetLeavesByHash(ctx, in)
}

// GetEntryAndProof forwards requests.
func (c *MutatingLogClient) GetEntryAndProof(ctx context.Context, in *trillian.GetEntryAndProofRequest, opts ...grpc.CallOption) (*trillian.GetEntryAndProofResponse, error) {
	return c.c.GetEntryAndProof(ctx, in)
}

// InitLog forwards requests.
func (c *MutatingLogClient) InitLog(ctx context.Context, in *trillian.InitLogRequest, opts ...grpc.CallOption) (*trillian.InitLogResponse, error) {
	return c.c.InitLog(ctx, in)
}

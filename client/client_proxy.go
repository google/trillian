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
	"math/rand"

	context "golang.org/x/net/context"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"google.golang.org/grpc"
)

// MockLogClient supports applying mutations to the return values of the TrillianLogClient
type MockLogClient struct {
	c                    trillian.TrillianLogClient
	mGetInclusionProof   bool
	mGetConsistencyProof bool
}

// QueueLeaf forwards requests.
func (c *MockLogClient) QueueLeaf(ctx context.Context, in *trillian.QueueLeafRequest, opts ...grpc.CallOption) (*google_protobuf.Empty, error) {
	return c.c.QueueLeaf(ctx, in)
}

// QueueLeaves forwards requests.
func (c *MockLogClient) QueueLeaves(ctx context.Context, in *trillian.QueueLeavesRequest, opts ...grpc.CallOption) (*trillian.QueueLeavesResponse, error) {
	return c.c.QueueLeaves(ctx, in)
}

// GetInclusionProof forwards requests and modifies the response.
func (c *MockLogClient) GetInclusionProof(ctx context.Context, in *trillian.GetInclusionProofRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofResponse, error) {
	resp, err := c.c.GetInclusionProof(ctx, in)
	if c.mGetInclusionProof {
		i := rand.Intn(len(resp.Proof.ProofNode))
		j := rand.Intn(len(resp.Proof.ProofNode[i].NodeHash))
		resp.Proof.ProofNode[i].NodeHash[j] ^= 4
	}
	return resp, err
}

// GetInclusionProofByHash forwards requests.
func (c *MockLogClient) GetInclusionProofByHash(ctx context.Context, in *trillian.GetInclusionProofByHashRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofByHashResponse, error) {
	resp, err := c.c.GetInclusionProofByHash(ctx, in)
	if c.mGetInclusionProof {
		h := rand.Intn(len(resp.Proof))
		i := rand.Intn(len(resp.Proof[h].ProofNode))
		j := rand.Intn(len(resp.Proof[h].ProofNode[i].NodeHash))
		resp.Proof[h].ProofNode[i].NodeHash[j] ^= 4
	}
	return resp, err
}

// GetConsistencyProof forwards requests and modifies responses.
func (c *MockLogClient) GetConsistencyProof(ctx context.Context, in *trillian.GetConsistencyProofRequest, opts ...grpc.CallOption) (*trillian.GetConsistencyProofResponse, error) {
	resp, err := c.c.GetConsistencyProof(ctx, in)
	if c.mGetConsistencyProof {
		i := rand.Intn(len(resp.Proof.ProofNode))
		j := rand.Intn(len(resp.Proof.ProofNode[i].NodeHash))
		resp.Proof.ProofNode[i].NodeHash[j] ^= 4
	}
	return resp, err
}

// GetLatestSignedLogRoot forwards requests.
func (c *MockLogClient) GetLatestSignedLogRoot(ctx context.Context, in *trillian.GetLatestSignedLogRootRequest, opts ...grpc.CallOption) (*trillian.GetLatestSignedLogRootResponse, error) {
	return c.c.GetLatestSignedLogRoot(ctx, in)
}

// GetSequencedLeafCount forwards requests.
func (c *MockLogClient) GetSequencedLeafCount(ctx context.Context, in *trillian.GetSequencedLeafCountRequest, opts ...grpc.CallOption) (*trillian.GetSequencedLeafCountResponse, error) {
	return c.c.GetSequencedLeafCount(ctx, in)
}

// GetLeavesByIndex forwards requests.
func (c *MockLogClient) GetLeavesByIndex(ctx context.Context, in *trillian.GetLeavesByIndexRequest, opts ...grpc.CallOption) (*trillian.GetLeavesByIndexResponse, error) {
	return c.c.GetLeavesByIndex(ctx, in)
}

// GetLeavesByHash forwards requests.
func (c *MockLogClient) GetLeavesByHash(ctx context.Context, in *trillian.GetLeavesByHashRequest, opts ...grpc.CallOption) (*trillian.GetLeavesByHashResponse, error) {
	return c.c.GetLeavesByHash(ctx, in)
}

// GetEntryAndProof forwards requests.
func (c *MockLogClient) GetEntryAndProof(ctx context.Context, in *trillian.GetEntryAndProofRequest, opts ...grpc.CallOption) (*trillian.GetEntryAndProofResponse, error) {
	return c.c.GetEntryAndProof(ctx, in)
}

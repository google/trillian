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

// Package logproxy forwards Trillian Log Server requests to another server.
package logproxy

import (
	context "golang.org/x/net/context"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
)

// Proxy forwards requests to the given TrillianLogClient
type Proxy struct {
	c trillian.TrillianLogClient
}

// New returns a new proxy for the TrillianLog
func New(c trillian.TrillianLogClient) *Proxy {
	return &Proxy{c: c}
}

// QueueLeaf forwards requests.
func (p *Proxy) QueueLeaf(ctx context.Context, in *trillian.QueueLeafRequest) (*google_protobuf.Empty, error) {
	return p.c.QueueLeaf(ctx, in)
}

// QueueLeaves forwards requests.
func (p *Proxy) QueueLeaves(ctx context.Context, in *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	return p.c.QueueLeaves(ctx, in)
}

// GetInclusionProof forwards requests and modifies the response.
func (p *Proxy) GetInclusionProof(ctx context.Context, in *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	return p.c.GetInclusionProof(ctx, in)
}

// GetInclusionProofByHash forwards requests.
func (p *Proxy) GetInclusionProofByHash(ctx context.Context, in *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	return p.c.GetInclusionProofByHash(ctx, in)
}

// GetConsistencyProof forwards requests and modifies responses.
func (p *Proxy) GetConsistencyProof(ctx context.Context, in *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	return p.c.GetConsistencyProof(ctx, in)
}

// GetLatestSignedLogRoot forwards requests.
func (p *Proxy) GetLatestSignedLogRoot(ctx context.Context, in *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	return p.c.GetLatestSignedLogRoot(ctx, in)
}

// GetSequencedLeafCount forwards requests.
func (p *Proxy) GetSequencedLeafCount(ctx context.Context, in *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	return p.c.GetSequencedLeafCount(ctx, in)
}

// GetLeavesByIndex forwards requests.
func (p *Proxy) GetLeavesByIndex(ctx context.Context, in *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	return p.c.GetLeavesByIndex(ctx, in)
}

// GetLeavesByHash forwards requests.
func (p *Proxy) GetLeavesByHash(ctx context.Context, in *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return p.c.GetLeavesByHash(ctx, in)
}

// GetEntryAndProof forwards requests.
func (p *Proxy) GetEntryAndProof(ctx context.Context, in *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	return p.c.GetEntryAndProof(ctx, in)
}

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

// Package proxy forwards Trillian Log Server requests to another server.
package proxy

import (
	"context"

	"github.com/google/trillian"
)

// Log implements the TrillianLogServer interface.
//
// For each RPC, Log forwards the request to the associated method in
// the TrillianLogClient.
type Log struct {
	c trillian.TrillianLogClient
}

// NewLog returns a new proxy for the TrillianLogServer.
func NewLog(c trillian.TrillianLogClient) *Log {
	return &Log{c: c}
}

// QueueLeaf forwards the RPC.
func (p *Log) QueueLeaf(ctx context.Context, in *trillian.QueueLeafRequest) (*trillian.QueueLeafResponse, error) {
	return p.c.QueueLeaf(ctx, in)
}

// QueueLeaves forwards the RPC.
func (p *Log) QueueLeaves(ctx context.Context, in *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	return p.c.QueueLeaves(ctx, in)
}

// AddSequencedLeaf forwards the RPC.
func (p *Log) AddSequencedLeaf(ctx context.Context, in *trillian.AddSequencedLeafRequest) (*trillian.AddSequencedLeafResponse, error) {
	return p.c.AddSequencedLeaf(ctx, in)
}

// AddSequencedLeaves forwards the RPC.
func (p *Log) AddSequencedLeaves(ctx context.Context, in *trillian.AddSequencedLeavesRequest) (*trillian.AddSequencedLeavesResponse, error) {
	return p.c.AddSequencedLeaves(ctx, in)
}

// GetInclusionProof forwards the RPC.
func (p *Log) GetInclusionProof(ctx context.Context, in *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	return p.c.GetInclusionProof(ctx, in)
}

// GetInclusionProofByHash forwards the RPC.
func (p *Log) GetInclusionProofByHash(ctx context.Context, in *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	return p.c.GetInclusionProofByHash(ctx, in)
}

// GetConsistencyProof forwards the RPC.
func (p *Log) GetConsistencyProof(ctx context.Context, in *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	return p.c.GetConsistencyProof(ctx, in)
}

// GetLatestSignedLogRoot forwards the RPC.
func (p *Log) GetLatestSignedLogRoot(ctx context.Context, in *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	return p.c.GetLatestSignedLogRoot(ctx, in)
}

// GetSequencedLeafCount forwards the RPC.
func (p *Log) GetSequencedLeafCount(ctx context.Context, in *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	return p.c.GetSequencedLeafCount(ctx, in)
}

// GetLeavesByIndex forwards the RPC.
func (p *Log) GetLeavesByIndex(ctx context.Context, in *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	return p.c.GetLeavesByIndex(ctx, in)
}

// GetLeavesByRange forwards the RPC.
func (p *Log) GetLeavesByRange(ctx context.Context, in *trillian.GetLeavesByRangeRequest) (*trillian.GetLeavesByRangeResponse, error) {
	return p.c.GetLeavesByRange(ctx, in)
}

// GetLeavesByHash forwards the RPC.
func (p *Log) GetLeavesByHash(ctx context.Context, in *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return p.c.GetLeavesByHash(ctx, in)
}

// GetEntryAndProof forwards the RPC.
func (p *Log) GetEntryAndProof(ctx context.Context, in *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	return p.c.GetEntryAndProof(ctx, in)
}

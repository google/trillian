// Copyright 2017 Google LLC. All Rights Reserved.
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
	"github.com/google/trillian/types"
	"google.golang.org/grpc"
)

// MutatingLogClient supports applying mutations to the return values of the TrillianLogClient
// for testing.
type MutatingLogClient struct {
	trillian.TrillianLogClient
	mutateInclusionProof   bool
	mutateConsistencyProof bool
	mutateRootSize         bool
}

// GetLatestSignedLogRoot forwards requests and optionally modifies the returned size.
func (c *MutatingLogClient) GetLatestSignedLogRoot(ctx context.Context, in *trillian.GetLatestSignedLogRootRequest, opts ...grpc.CallOption) (*trillian.GetLatestSignedLogRootResponse, error) {
	resp, err := c.TrillianLogClient.GetLatestSignedLogRoot(ctx, in)
	if c.mutateRootSize {
		var root types.LogRootV1
		if err := root.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
			panic("failed to unmarshal")
		}
		root.TreeSize += 10000
		resp.SignedLogRoot.LogRoot, _ = root.MarshalBinary()
	}
	return resp, err
}

// GetInclusionProof forwards requests and optionally corrupts the response.
func (c *MutatingLogClient) GetInclusionProof(ctx context.Context, in *trillian.GetInclusionProofRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofResponse, error) {
	resp, err := c.TrillianLogClient.GetInclusionProof(ctx, in)
	if c.mutateInclusionProof {
		i := rand.Intn(len(resp.Proof.Hashes))
		j := rand.Intn(len(resp.Proof.Hashes[i]))
		resp.Proof.Hashes[i][j] ^= 4
	}
	return resp, err
}

// GetInclusionProofByHash forwards requests and optionaly corrupts responses.
func (c *MutatingLogClient) GetInclusionProofByHash(ctx context.Context, in *trillian.GetInclusionProofByHashRequest, opts ...grpc.CallOption) (*trillian.GetInclusionProofByHashResponse, error) {
	resp, err := c.TrillianLogClient.GetInclusionProofByHash(ctx, in)
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
	resp, err := c.TrillianLogClient.GetConsistencyProof(ctx, in)
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

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

// Package client verifies responses from the Trillian log.
package client

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client/backoff"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// LogClient represents a client for a given Trillian log instance.
type LogClient struct {
	LogID  int64
	client trillian.TrillianLogClient
	hasher merkle.TreeHasher
	root   trillian.SignedLogRoot
	pubKey gocrypto.PublicKey
}

// New returns a new LogClient.
func New(logID int64, client trillian.TrillianLogClient, hasher merkle.TreeHasher, pubKey gocrypto.PublicKey) VerifyingLogClient {
	return &LogClient{
		LogID:  logID,
		client: client,
		hasher: hasher,
		pubKey: pubKey,
	}
}

// Root returns the last valid root seen by UpdateRoot.
// Returns an empty SignedLogRoot if UpdateRoot has not been called.
func (c *LogClient) Root() trillian.SignedLogRoot {
	return c.root
}

// AddLeaf adds leaf to the append only log.
// Blocks until it gets a verifiable response.
func (c *LogClient) AddLeaf(ctx context.Context, data []byte) error {
	// Fetch the current Root so we can detect when we update.
	if err := c.UpdateRoot(ctx); err != nil {
		return err
	}

	leaf := c.buildLeaf(data)
	err := c.queueLeaf(ctx, leaf)
	switch {
	case grpc.Code(err) == codes.AlreadyExists:
		// If the leaf already exists, don't wait for an update.
		return c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.root.TreeSize)
	case err != nil:
		return err
	default:
		err := grpc.Errorf(codes.NotFound, "Pre-loop condition")
		for i := 0; grpc.Code(err) == codes.NotFound && ctx.Err() == nil; i++ {
			// Wait for TreeSize to update.
			if err := c.waitForRootUpdate(ctx); err != nil {
				return err
			}

			// Get proof by hash.
			err = c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.root.TreeSize)
		}
		return err
	}
}

// waitForRootUpdate repeatedly fetches the Root until the TreeSize changes
// or until ctx times out.
func (c *LogClient) waitForRootUpdate(ctx context.Context) error {
	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	startTreeSize := c.root.TreeSize
	for i := 0; ; i++ {
		if err := c.UpdateRoot(ctx); err != nil {
			return err
		}
		if c.root.TreeSize > startTreeSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return grpc.Errorf(codes.DeadlineExceeded,
				"%v. TreeSize: %v, want > %v. Tried %v times.",
				err, c.root.TreeSize, startTreeSize, i+1)
		}
		time.Sleep(b.Duration())
	}
}

// UpdateRoot retrieves the current SignedLogRoot.
// Verifies the signature, and the consistency proof if this is not the first root this client has seen.
func (c *LogClient) UpdateRoot(ctx context.Context) error {
	req := &trillian.GetLatestSignedLogRootRequest{
		LogId: c.LogID,
	}
	resp, err := c.client.GetLatestSignedLogRoot(ctx, req)
	if err != nil {
		return err
	}
	str := resp.SignedLogRoot

	// Verify SignedLogRoot signature.
	hash := crypto.HashLogRoot(*str)
	if err := crypto.Verify(c.pubKey, hash, str.Signature); err != nil {
		return err
	}

	// Verify Consistency proof.
	if str.TreeSize == c.root.TreeSize &&
		bytes.Equal(str.RootHash, c.root.RootHash) {
		// Tree has not been updated.
		return nil
	}

	// Implicitly trust the first root we get.
	if c.root.TreeSize != 0 {
		// Get consistency proof.
		req := &trillian.GetConsistencyProofRequest{
			LogId:          c.LogID,
			FirstTreeSize:  c.root.TreeSize,
			SecondTreeSize: str.TreeSize,
		}
		proof, err := c.client.GetConsistencyProof(ctx, req)
		if err != nil {
			return err
		}
		// Verify consistency proof.
		v := merkle.NewLogVerifier(c.hasher)
		if err := v.VerifyConsistencyProof(
			c.root.TreeSize, str.TreeSize,
			c.root.RootHash, str.RootHash,
			convertProof(proof.Proof)); err != nil {
			return err
		}
	}
	c.root = *str
	return nil
}

func (c *LogClient) getInclusionProof(ctx context.Context, leafHash []byte, treeSize int64) error {
	req := &trillian.GetInclusionProofByHashRequest{
		LogId:    c.LogID,
		LeafHash: leafHash,
		TreeSize: treeSize,
	}
	resp, err := c.client.GetInclusionProofByHash(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Proof) < 1 {
		return errors.New("no inclusion proof supplied")
	}
	for _, proof := range resp.Proof {
		neighbors := convertProof(proof)
		v := merkle.NewLogVerifier(c.hasher)
		if err := v.VerifyInclusionProof(proof.LeafIndex, treeSize, neighbors, c.root.RootHash, leafHash); err != nil {
			return err
		}
	}
	return nil
}

// convertProof returns a slice of neighbor nodes from a trillian Proof.
// TODO(martin): adjust the public API to do this in the server before returing a proof.
func convertProof(proof *trillian.Proof) [][]byte {
	neighbors := make([][]byte, len(proof.ProofNode))
	for i, node := range proof.ProofNode {
		neighbors[i] = node.NodeHash
	}
	return neighbors
}

func (c *LogClient) buildLeaf(data []byte) *trillian.LogLeaf {
	hash := sha256.Sum256(data)
	leaf := &trillian.LogLeaf{
		LeafValue:        data,
		MerkleLeafHash:   c.hasher.HashLeaf(data),
		LeafIdentityHash: hash[:],
	}
	return leaf
}

func (c *LogClient) queueLeaf(ctx context.Context, leaf *trillian.LogLeaf) error {
	// Queue Leaf
	req := trillian.QueueLeafRequest{
		LogId: c.LogID,
		Leaf:  leaf,
	}
	rsp, err := c.client.QueueLeaf(ctx, &req)
	if err != nil {
		return err
	}
	if rsp.QueuedLeaf.Status != nil && rsp.QueuedLeaf.Status.Code == int32(code.Code_ALREADY_EXISTS) {
		// Convert this to AlreadyExists
		return grpc.Errorf(codes.AlreadyExists, "leaf already exists")
	}
	return nil
}

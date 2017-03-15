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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// LogClient represents a client for a given Trillian log instance.
type LogClient struct {
	LogID    int64
	client   trillian.TrillianLogClient
	hasher   merkle.TreeHasher
	STR      trillian.SignedLogRoot
	MaxTries int
	pubKey   gocrypto.PublicKey
}

// New returns a new LogClient.
func New(logID int64, client trillian.TrillianLogClient, hasher merkle.TreeHasher, pubKey gocrypto.PublicKey) *LogClient {
	return &LogClient{
		LogID:    logID,
		client:   client,
		hasher:   hasher,
		MaxTries: 3,
		pubKey:   pubKey,
	}
}

// AddLeaf adds leaf to the append only log.
// Blocks until it gets a verifiable response.
func (c *LogClient) AddLeaf(ctx context.Context, data []byte) error {
	// Fetch the current STR so we can detect when we update.
	if err := c.UpdateSTR(ctx); err != nil {
		return err
	}

	leaf := c.buildLeaf(data)
	err := c.queueLeaf(ctx, leaf)
	switch {
	case grpc.Code(err) == codes.AlreadyExists:
		// If the leaf already exists, don't wait for an update.
		return c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.STR.TreeSize)
	case err != nil:
		return err
	default:
		err := grpc.Errorf(codes.NotFound, "Pre-loop condition")
		for i := 0; grpc.Code(err) == codes.NotFound && i < c.MaxTries; i++ {
			// Wait for TreeSize to update.
			if err := c.waitForSTRUpdate(ctx, c.MaxTries); err != nil {
				return err
			}

			// Get proof by hash.
			err = c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.STR.TreeSize)
		}
		return err
	}
}

// waitForSTRUpdate repeatedly fetches the STR until the TreeSize changes
// or until a maximum number of attempts has been tried.
func (c *LogClient) waitForSTRUpdate(ctx context.Context, attempts int) error {
	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	startTreeSize := c.STR.TreeSize
	for i := 0; ; i++ {
		if err := c.UpdateSTR(ctx); err != nil {
			return err
		}
		if c.STR.TreeSize > startTreeSize {
			return nil
		}
		if i >= (attempts - 1) {
			return grpc.Errorf(codes.DeadlineExceeded,
				"UpdateSTR().TreeSize: %v, want > %v. Tried %v times.",
				c.STR.TreeSize, startTreeSize, i+1)
		}
		time.Sleep(b.Duration())
	}
}

// UpdateSTR retrieves the current SignedLogRoot and verifies it.
func (c *LogClient) UpdateSTR(ctx context.Context) error {
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
	if str.TreeSize == c.STR.TreeSize && bytes.Equal(str.RootHash, c.STR.RootHash) {
		// Tree has not been updated.
		return nil
	}

	// Implicitly trust the first STH we get.
	if c.STR.TreeSize != 0 {
		// Get consistency proof.
		req := &trillian.GetConsistencyProofRequest{
			LogId:          c.LogID,
			FirstTreeSize:  c.STR.TreeSize,
			SecondTreeSize: str.TreeSize,
		}
		proof, err := c.client.GetConsistencyProof(ctx, req)
		if err != nil {
			return err
		}
		// Verify consistency proof.
		v := merkle.NewLogVerifier(c.hasher)
		if err := v.VerifyConsistencyProof(
			c.STR.TreeSize, str.TreeSize,
			c.STR.RootHash, str.RootHash,
			convertProof(proof.Proof)); err != nil {
			return err
		}
	}
	c.STR = *str
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
		if err := v.VerifyInclusionProof(proof.LeafIndex, treeSize, neighbors, c.STR.RootHash, leafHash); err != nil {
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
	_, err := c.client.QueueLeaf(ctx, &req)
	return err
}

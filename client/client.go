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
	"crypto"
	"errors"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client/backoff"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LogClient represents a client for a given Trillian log instance.
type LogClient struct {
	LogID  int64
	client trillian.TrillianLogClient
	*logVerifier
	root trillian.SignedLogRoot
}

// New returns a new LogClient.
func New(logID int64, client trillian.TrillianLogClient, hasher hashers.LogHasher, pubKey crypto.PublicKey) *LogClient {
	return &LogClient{
		LogID:  logID,
		client: client,
		logVerifier: &logVerifier{
			hasher: hasher,
			pubKey: pubKey,
			v:      merkle.NewLogVerifier(hasher),
		},
	}
}

// NewFromTree creates a new LogClient given a tree config.
func NewFromTree(client trillian.TrillianLogClient, config *trillian.Tree) (*LogClient, error) {
	if got, want := config.TreeType, trillian.TreeType_LOG; got != want {
		return nil, fmt.Errorf("client: NewFromTree(): TreeType: %v, want %v", got, want)
	}
	// Log Hasher.
	logHasher, err := hashers.NewLogHasher(config.GetHashStrategy())
	if err != nil {
		return nil, fmt.Errorf("client: NewFromTree(): NewLogHasher(): %v", err)
	}

	// Log Key
	logPubKey, err := der.UnmarshalPublicKey(config.GetPublicKey().GetDer())
	if err != nil {
		return nil, fmt.Errorf("client: NewFromTree(): Failed parsing Log public key: %v", err)
	}

	logID := config.GetTreeId()

	return New(logID, client, logHasher, logPubKey), nil
}

// AddLeaf adds leaf to the append only log.
// Blocks until it gets a verifiable response.
func (c *LogClient) AddLeaf(ctx context.Context, data []byte) error {
	if err := c.QueueLeaf(ctx, data); err != nil {
		return fmt.Errorf("QueueLeaf(): %v", err)
	}
	if err := c.WaitForInclusion(ctx, data); err != nil {
		return fmt.Errorf("WaitForInclusion(): %v", err)
	}
	return nil
}

// GetByIndex returns a single leaf at the requested index.
func (c *LogClient) GetByIndex(ctx context.Context, index int64) (*trillian.LogLeaf, error) {
	resp, err := c.client.GetLeavesByIndex(ctx, &trillian.GetLeavesByIndexRequest{
		LogId:     c.LogID,
		LeafIndex: []int64{index},
	})
	if err != nil {
		return nil, err
	}
	if got, want := len(resp.Leaves), 1; got != want {
		return nil, fmt.Errorf("len(leaves): %v, want %v", got, want)
	}
	return resp.Leaves[0], nil
}

// ListByIndex returns the requested leaves by index.
func (c *LogClient) ListByIndex(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	resp, err := c.client.GetLeavesByRange(ctx,
		&trillian.GetLeavesByRangeRequest{
			LogId:      c.LogID,
			StartIndex: start,
			Count:      count,
		})
	if err != nil {
		return nil, err
	}
	// Verify that we got back the requested leaves.
	if len(resp.Leaves) < int(count) {
		return nil, fmt.Errorf("len(Leaves)=%d, want %d", len(resp.Leaves), count)
	}
	for i, l := range resp.Leaves {
		if want := start + int64(i); l.LeafIndex != want {
			return nil, fmt.Errorf("Leaves[%d].LeafIndex=%d, want %d", i, l.LeafIndex, want)
		}
	}

	return resp.Leaves, nil
}

// WaitForRootUpdate repeatedly fetches the Root until the fetched tree size >=
// waitForTreeSize or until ctx times out.
func (c *LogClient) WaitForRootUpdate(ctx context.Context, waitForTreeSize int64) (*trillian.SignedLogRoot, error) {
	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	for i := 0; ; i++ {
		root, err := c.UpdateRoot(ctx)
		switch x := status.Code(err); x {
		case codes.OK:
			if root.TreeSize >= waitForTreeSize {
				return root, nil
			}
		case codes.Unavailable, codes.NotFound: // Retry.
		default:
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, status.Errorf(codes.DeadlineExceeded,
				"%v. TreeSize: %v, want >= %v. Tried %v times: %v",
				err, c.root.TreeSize, waitForTreeSize, i+1, ctx.Err())
		case <-time.After(b.Duration()):
		}
	}
}

// getLatestRoot fetches and verifies the latest root against a trusted root, seen in the past.
// Pass nil for trusted if this is the first time querying this log.
func (c *LogClient) getLatestRoot(ctx context.Context, trusted *trillian.SignedLogRoot) (*trillian.SignedLogRoot, error) {
	resp, err := c.client.GetLatestSignedLogRoot(ctx,
		&trillian.GetLatestSignedLogRootRequest{
			LogId: c.LogID,
		})
	if err != nil {
		return nil, err
	}
	if trusted.TreeSize > 0 &&
		resp.SignedLogRoot.TreeSize == trusted.TreeSize &&
		bytes.Equal(resp.SignedLogRoot.RootHash, trusted.RootHash) {
		// Tree has not been updated.
		return resp.SignedLogRoot, nil
	}
	// Fetch a consistency proof if this isn't the first root we've seen.
	var consistency *trillian.GetConsistencyProofResponse
	if trusted.TreeSize > 0 {
		// Get consistency proof.
		consistency, err = c.client.GetConsistencyProof(ctx,
			&trillian.GetConsistencyProofRequest{
				LogId:          c.LogID,
				FirstTreeSize:  trusted.TreeSize,
				SecondTreeSize: resp.SignedLogRoot.TreeSize,
			})
		if err != nil {
			return nil, err
		}
	}
	// Verify root update if the tree / the latest signed log root isn't empty.
	if resp.GetSignedLogRoot().GetTreeSize() > 0 {
		if err := c.logVerifier.VerifyRoot(trusted, resp.GetSignedLogRoot(),
			consistency.GetProof().GetHashes()); err != nil {
			return nil, err
		}
	}
	return resp.SignedLogRoot, nil
}

// UpdateRoot retrieves the current SignedLogRoot, verifying it against roots this client has
// seen in the past, and updating the currently trusted root if the new root verifies.
func (c *LogClient) UpdateRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	currentlyTrusted := &c.root
	newTrusted, err := c.getLatestRoot(ctx, currentlyTrusted)
	if err != nil {
		return nil, err
	}
	if newTrusted.TimestampNanos > currentlyTrusted.TimestampNanos &&
		newTrusted.TreeSize >= currentlyTrusted.TreeSize {
		c.root = *newTrusted
	}
	// Copy the internal trusted root in order to prevent clients from modifying it.
	ret := c.root
	return &ret, nil
}

// WaitForInclusion blocks until the requested data has been verified with an inclusion proof.
// This assumes that the data has already been submitted.
// Best practice is to call this method with a context that will timeout.
func (c *LogClient) WaitForInclusion(ctx context.Context, data []byte) error {
	leaf, err := c.logVerifier.buildLeaf(data)
	if err != nil {
		return err
	}

	// Fetch the current Root to improve our chances at a valid inclusion proof.
	root, err := c.UpdateRoot(ctx)
	if err != nil {
		return err
	}
	if root.TreeSize == 0 {
		// If the TreeSize is 0, wait for something to be in the log.
		// It is illegal to ask for an inclusion proof with TreeSize = 0.
		if _, err := c.WaitForRootUpdate(ctx, 1); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := c.getInclusionProof(ctx, leaf.MerkleLeafHash, root.TreeSize)
		s, ok := status.FromError(err)
		if !ok {
			return err
		}
		switch s.Code() {
		case codes.OK:
			return nil
		case codes.NotFound:
			// Wait for TreeSize to update.
			if _, err := c.WaitForRootUpdate(ctx, c.root.TreeSize+1); err != nil {
				return err
			}
		default:
			return err
		}
	}
}

// VerifyInclusion updates the log root and ensures that the given leaf data has been included in the log.
func (c *LogClient) VerifyInclusion(ctx context.Context, data []byte) error {
	leaf, err := c.logVerifier.buildLeaf(data)
	if err != nil {
		return err
	}
	root, err := c.UpdateRoot(ctx)
	if err != nil {
		return fmt.Errorf("UpdateRoot(): %v", err)
	}
	return c.getInclusionProof(ctx, leaf.MerkleLeafHash, root.TreeSize)
}

// VerifyInclusionAtIndex updates the log root and ensures that the given leaf data has been included in the log at a particular index.
func (c *LogClient) VerifyInclusionAtIndex(ctx context.Context, data []byte, index int64) error {
	root, err := c.UpdateRoot(ctx)
	if err != nil {
		return fmt.Errorf("UpdateRoot(): %v", err)
	}
	resp, err := c.client.GetInclusionProof(ctx,
		&trillian.GetInclusionProofRequest{
			LogId:     c.LogID,
			LeafIndex: index,
			TreeSize:  root.TreeSize,
		})
	if err != nil {
		return err
	}
	return c.logVerifier.VerifyInclusionAtIndex(root, data, index, resp.Proof.Hashes)
}

func (c *LogClient) getInclusionProof(ctx context.Context, leafHash []byte, treeSize int64) error {
	resp, err := c.client.GetInclusionProofByHash(ctx,
		&trillian.GetInclusionProofByHashRequest{
			LogId:    c.LogID,
			LeafHash: leafHash,
			TreeSize: treeSize,
		})
	if err != nil {
		return err
	}
	if len(resp.Proof) < 1 {
		return errors.New("no inclusion proof supplied")
	}
	for _, proof := range resp.Proof {
		if err := c.logVerifier.VerifyInclusionByHash(&c.root, leafHash, proof); err != nil {
			return err
		}
	}
	return nil
}

// QueueLeaf adds a leaf to a Trillian log without blocking.
// AlreadyExists is considered a success case by this function.
func (c *LogClient) QueueLeaf(ctx context.Context, data []byte) error {
	leaf, err := c.logVerifier.buildLeaf(data)
	if err != nil {
		return err
	}

	if _, err := c.client.QueueLeaf(ctx, &trillian.QueueLeafRequest{
		LogId: c.LogID,
		Leaf:  leaf,
	}); err != nil {
		return err
	}
	return nil
}

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
	"sort"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client/backoff"
	"github.com/google/trillian/merkle"
	"google.golang.org/genproto/googleapis/rpc/code"
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
func New(logID int64, client trillian.TrillianLogClient, hasher merkle.TreeHasher, pubKey crypto.PublicKey) *LogClient {
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

	leaf := c.logVerifier.buildLeaf(data)
	err := c.queueLeaf(ctx, leaf)
	switch s, ok := status.FromError(err); {
	case ok && s.Code() == codes.AlreadyExists:
		// If the leaf already exists, don't wait for an update.
		return c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.root.TreeSize)
	case err != nil:
		return err
	default:
		err := status.Errorf(codes.NotFound, "Pre-loop condition")
		for {
			if ctx.Err() != nil {
				break
			}
			if s, ok := status.FromError(err); !ok || s.Code() != codes.NotFound {
				break
			}

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
	indexes := make([]int64, count)
	for i := range indexes {
		indexes[i] = start + int64(i)
	}

	resp, err := c.client.GetLeavesByIndex(ctx,
		&trillian.GetLeavesByIndexRequest{
			LogId:     c.LogID,
			LeafIndex: indexes,
		})
	if err != nil {
		return nil, err
	}
	// Responses are not required to be in-order.
	sort.Sort(byLeafIndex(resp.Leaves))
	// Verify that we got back the requested leaves.
	if got, want := len(resp.Leaves), len(indexes); got != want {
		return nil, fmt.Errorf("len(Leaves): %v, want %v", got, want)
	}
	for i, l := range resp.Leaves {
		if got, want := l.LeafIndex, indexes[i]; got != want {
			return nil, fmt.Errorf("Leaves[%v].Index: %v, want %v", i, got, want)
		}
	}

	return resp.Leaves, nil
}

type byLeafIndex []*trillian.LogLeaf

func (ll byLeafIndex) Len() int           { return len(ll) }
func (ll byLeafIndex) Swap(i, j int)      { ll[i], ll[j] = ll[j], ll[i] }
func (ll byLeafIndex) Less(i, j int) bool { return ll[i].LeafIndex < ll[j].LeafIndex }

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
			return status.Errorf(codes.DeadlineExceeded,
				"%v. TreeSize: %v, want > %v. Tried %v times.",
				err, c.root.TreeSize, startTreeSize, i+1)
		}
		time.Sleep(b.Duration())
	}
}

// UpdateRoot retrieves the current SignedLogRoot.
// Verifies the signature, and the consistency proof if this is not the first root this client has seen.
func (c *LogClient) UpdateRoot(ctx context.Context) error {
	resp, err := c.client.GetLatestSignedLogRoot(ctx,
		&trillian.GetLatestSignedLogRootRequest{
			LogId: c.LogID,
		})
	if err != nil {
		return err
	}
	if c.root.TreeSize > 0 &&
		resp.SignedLogRoot.TreeSize == c.root.TreeSize &&
		bytes.Equal(resp.SignedLogRoot.RootHash, c.root.RootHash) {
		// Tree has not been updated.
		return nil
	}
	// Fetch a consistency proof if this isn't the first root we've seen.
	var consistency *trillian.GetConsistencyProofResponse
	if c.root.TreeSize != 0 {
		// Get consistency proof.
		consistency, err = c.client.GetConsistencyProof(ctx,
			&trillian.GetConsistencyProofRequest{
				LogId:          c.LogID,
				FirstTreeSize:  c.root.TreeSize,
				SecondTreeSize: resp.SignedLogRoot.TreeSize,
			})
		if err != nil {
			return err
		}
	}

	// Verify root update.
	if err := c.logVerifier.VerifyRoot(&c.root, resp.SignedLogRoot,
		consistency.GetProof().GetHashes()); err != nil {
		return err
	}
	c.root = *resp.SignedLogRoot
	return nil
}

// VerifyInclusion updates the log root and ensures that the given leaf data has been included in the log.
func (c *LogClient) VerifyInclusion(ctx context.Context, data []byte) error {
	leaf := c.logVerifier.buildLeaf(data)
	if err := c.UpdateRoot(ctx); err != nil {
		return fmt.Errorf("UpdateRoot(): %v", err)
	}
	return c.getInclusionProof(ctx, leaf.MerkleLeafHash, c.root.TreeSize)
}

// VerifyInclusionAtIndex updates the log root and ensures that the given leaf data has been included in the log at a particular index.
func (c *LogClient) VerifyInclusionAtIndex(ctx context.Context, data []byte, index int64) error {
	if err := c.UpdateRoot(ctx); err != nil {
		return fmt.Errorf("UpdateRoot(): %v", err)
	}
	resp, err := c.client.GetInclusionProof(ctx,
		&trillian.GetInclusionProofRequest{
			LogId:     c.LogID,
			LeafIndex: index,
			TreeSize:  c.root.TreeSize,
		})
	if err != nil {
		return err
	}
	return c.logVerifier.VerifyInclusionAtIndex(&c.root, data, index, resp.Proof.Hashes)
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
		return status.Errorf(codes.AlreadyExists, "leaf already exists")
	}
	return nil
}

// Copyright 2016 Google Inc. All Rights Reserved.
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
	"context"
	"crypto/sha256"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

// LogClient represents a client for a given Trillian log instance.
type LogClient struct {
	LogID  int64
	client trillian.TrillianLogClient
}

// New returns a new LogClient.
func New(logID int64, cc *grpc.ClientConn) *LogClient {
	return &LogClient{
		LogID:  logID,
		client: trillian.NewTrillianLogClient(cc),
	}
}

// AddLeaf adds leaf to the append only log. It blocks until a verifiable response is received.
func (c *LogClient) AddLeaf(data []byte) error {
	hash := sha256.Sum256(data)
	leaf := &trillian.LogLeaf{
		LeafValue:      data,
		MerkleLeafHash: hash[:],
		LeafValueHash:  hash[:],
	}
	req := trillian.QueueLeafRequest{
		LogId: c.LogID,
		Leaf:  leaf,
	}
	ctx := context.TODO()
	_, err := c.client.QueueLeaf(ctx, &req)
	// TODO(gdbelvin): Get proof by hash
	// TODO(gdbelvin): backoff with jitter
	// TODO(gdbelvin): verify proof
	return err
}

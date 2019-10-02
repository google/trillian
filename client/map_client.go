// Copyright 2018 Google Inc. All Rights Reserved.
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
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/status"
)

// MapClient represents a client for a given Trillian Map instance.
type MapClient struct {
	*MapVerifier
	MapID int64
	Conn  trillian.TrillianMapClient
}

// NewMapClientFromTree returns a verifying Map client for the specified tree.
func NewMapClientFromTree(client trillian.TrillianMapClient, config *trillian.Tree) (*MapClient, error) {
	verifier, err := NewMapVerifierFromTree(config)
	if err != nil {
		return nil, err
	}
	return &MapClient{
		MapVerifier: verifier,
		MapID:       config.TreeId,
		Conn:        client,
	}, nil
}

// GetAndVerifyLatestMapRoot verifies and returns the latest map root.
func (c *MapClient) GetAndVerifyLatestMapRoot(ctx context.Context) (*types.MapRootV1, error) {
	rootResp, err := c.Conn.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: c.MapID})
	if err != nil {
		s := status.Convert(err)
		return nil, status.Errorf(s.Code(), "GetSignedMapRoot(%v): %v", c.MapID, s.Message())
	}
	return c.VerifySignedMapRoot(rootResp.GetMapRoot())
}

// GetAndVerifyMapRootByRevision verifies and returns the map root with the given revision.
func (c *MapClient) GetAndVerifyMapRootByRevision(ctx context.Context, revision int64) (*types.MapRootV1, error) {
	rootResp, err := c.Conn.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{MapId: c.MapID, Revision: revision})
	if err != nil {
		s := status.Convert(err)
		return nil, status.Errorf(s.Code(), "GetSignedMapRootByRevision(%v, %d): %v", c.MapID, revision, s.Message())
	}
	root, err := c.VerifySignedMapRoot(rootResp.GetMapRoot())
	if err != nil {
		return nil, fmt.Errorf("GetAndVerifyMapRootByRevision(%v, %d) failed to verify root: %v", c.MapID, revision, err)
	}
	if int64(root.Revision) != revision {
		return nil, fmt.Errorf("GetAndVerifyMapRootByRevision(%v, %d): got revision %d", c.MapID, revision, root.Revision)
	}
	return root, err
}

// GetAndVerifyMapLeaves verifies and returns the requested map leaves.
// indexes may not contain duplicates.
func (c *MapClient) GetAndVerifyMapLeaves(ctx context.Context, indexes [][]byte) ([]*trillian.MapLeaf, error) {
	getResp, err := c.Conn.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
		MapId: c.MapID,
		Index: indexes,
	})
	if err != nil {
		s := status.Convert(err)
		return nil, status.Errorf(s.Code(), "map.GetLeaves(): %v", s.Message())
	}
	return c.VerifyMapLeavesResponse(indexes, -1, getResp)
}

// GetAndVerifyMapLeavesByRevision verifies and returns the requested map leaves at a specific revision.
// indexes may not contain duplicates.
func (c *MapClient) GetAndVerifyMapLeavesByRevision(ctx context.Context, revision int64, indexes [][]byte) ([]*trillian.MapLeaf, error) {
	getResp, err := c.Conn.GetLeavesByRevision(ctx, &trillian.GetMapLeavesByRevisionRequest{
		MapId:    c.MapID,
		Index:    indexes,
		Revision: revision,
	})
	if err != nil {
		s := status.Convert(err)
		return nil, status.Errorf(s.Code(), "map.GetLeaves(): %v", s.Message())
	}
	return c.VerifyMapLeavesResponse(indexes, revision, getResp)
}

// SetAndVerifyMapLeaves calls SetLeaves and verifies the signature of the returned map root.
// Deprecated: Use WriteLeaves on the TrillianMapWriteClient instead.
func (c *MapClient) SetAndVerifyMapLeaves(ctx context.Context, leaves []*trillian.MapLeaf, metadata []byte) (*types.MapRootV1, error) {
	// Set new leaf values.
	req := &trillian.SetMapLeavesRequest{
		MapId:    c.MapID,
		Leaves:   leaves,
		Metadata: metadata,
	}
	setResp, err := c.Conn.SetLeaves(ctx, req)
	if err != nil {
		s := status.Convert(err)
		return nil, status.Errorf(s.Code(), "map.SetLeaves(MapId: %v): %v", c.MapID, s.Message())
	}
	return c.VerifySignedMapRoot(setResp.GetMapRoot())
}

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

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
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
		return nil, status.Errorf(status.Code(err), "GetSignedMapRoot(%v): %v", c.MapID, err)
	}
	mapRoot, err := c.VerifySignedMapRoot(rootResp.GetMapRoot())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "VerifySignedMapRoot(%v): %v", c.MapID, err)
	}
	return mapRoot, nil
}

// GetAndVerifyMapLeaves verifies and returns the requested map leaves.
// indexes may not contain duplicates.
func (c *MapClient) GetAndVerifyMapLeaves(ctx context.Context, indexes [][]byte) ([]*trillian.MapLeaf, error) {
	if err := hasDuplicates(indexes); err != nil {
		return nil, err
	}
	getResp, err := c.Conn.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
		MapId: c.MapID,
		Index: indexes,
	})
	if err != nil {
		return nil, status.Errorf(status.Code(err), "map.GetLeaves(): %v", err)
	}
	return c.VerifyMapLeavesResponse(indexes, -1, getResp)
}

// GetAndVerifyMapLeavesByRevision verifies and returns the requested map leaves at a specific revision.
// indexes may not contain duplicates.
func (c *MapClient) GetAndVerifyMapLeavesByRevision(ctx context.Context, revision int64, indexes [][]byte) ([]*trillian.MapLeaf, error) {
	if err := hasDuplicates(indexes); err != nil {
		return nil, err
	}
	getResp, err := c.Conn.GetLeavesByRevision(ctx, &trillian.GetMapLeavesByRevisionRequest{
		MapId:    c.MapID,
		Index:    indexes,
		Revision: revision,
	})
	if err != nil {
		return nil, status.Errorf(status.Code(err), "map.GetLeaves(): %v", err)
	}
	return c.VerifyMapLeavesResponse(indexes, revision, getResp)
}

// hasDuplicates returns an error if there are duplicates in indexes.
func hasDuplicates(indexes [][]byte) error {
	set := make(map[string]bool)
	for _, i := range indexes {
		if set[string(i)] {
			return status.Errorf(codes.InvalidArgument,
				"map.GetLeaves(): index %x requested more than once", i)
		}
		set[string(i)] = true
	}
	return nil
}

// SetAndVerifyMapLeaves calls SetLeaves and verifies the signature of the returned map root.
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

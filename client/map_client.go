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

	tpb "github.com/google/trillian"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MapClient represents a client for a given Trillian log instance.
type MapClient struct {
	*MapVerifier
	MapID int64
	Conn  tpb.TrillianMapClient
}

// NewMapClientFromTree returns a verifying map client.
func NewMapClientFromTree(client tpb.TrillianMapClient, config *tpb.Tree) (*MapClient, error) {
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
	rootResp, err := c.Conn.GetSignedMapRoot(ctx, &tpb.GetSignedMapRootRequest{MapId: c.MapID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetSignedMapRoot(%v): %v", c.MapID, err)
	}
	mapRoot, err := c.VerifySignedMapRoot(rootResp.GetMapRoot())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "VerifySignedMapRoot(): %v", err)
	}
	return mapRoot, nil
}

// GetAndVerifyMapLeavesAtRevision verifies and returns the requested map leaves.
func (c *MapClient) GetAndVerifyMapLeavesAtRevision(ctx context.Context, mapRoot *types.MapRootV1, indexes [][]byte) ([]*tpb.MapLeaf, error) {
	getResp, err := c.Conn.GetLeaves(ctx, &tpb.GetMapLeavesRequest{
		MapId: c.MapID,
		Index: indexes,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "tmap.GetLeaves(): %v", err)
	}
	if got, want := len(getResp.MapLeafInclusion), len(indexes); got != want {
		return nil, status.Errorf(codes.Internal, "got %v leaves, want %v", got, want)
	}
	leaves := make([]*tpb.MapLeaf, 0, len(getResp.MapLeafInclusion))
	for _, m := range getResp.MapLeafInclusion {
		if err := c.VerifyMapLeafInclusionHash(mapRoot.RootHash, m); err != nil {
			return nil, status.Errorf(codes.Internal, "map: VerifyMapLeafInclusion(): %v", err)
		}
		leaves = append(leaves, m.Leaf)
	}
	return leaves, nil
}

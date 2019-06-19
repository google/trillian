// Copyright 2019 Google Inc. All Rights Reserved.
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

package server

import (
	"context"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/trees"
)

// TrillianMapWriteServer implements the Write RPC API
type TrillianMapWriteServer struct {
	mapServer *TrillianMapServer
	registry  extension.Registry
}

// NewTrillianMapWriteServer creates a new RPC server for map writes
func NewTrillianMapWriteServer(registry extension.Registry, mapServer *TrillianMapServer) *TrillianMapWriteServer {
	return &TrillianMapWriteServer{mapServer: mapServer, registry: registry}
}

// GetLeavesByRevision implements the GetLeavesByRevision write RPC method.
func (t *TrillianMapWriteServer) GetLeavesByRevision(ctx context.Context, req *trillian.GetMapLeavesByRevisionRequest) (*trillian.MapLeaves, error) {
	return t.mapServer.GetLeavesByRevisionNoProof(ctx, req)
}

// WriteLeaves implements the WriteLeaves write RPC method.
func (t *TrillianMapWriteServer) WriteLeaves(ctx context.Context, req *trillian.WriteMapLeavesRequest) (*trillian.WriteMapLeavesResponse, error) {
	tree, err := trees.GetTree(ctx, t.registry.AdminStorage, req.MapId, optsMapWrite)
	if err != nil {
		return nil, err
	}
	pub, err := der.UnmarshalPublicKey(tree.PublicKey.GetDer())
	if err != nil {
		return nil, err
	}
	hash, err := trees.Hash(tree)
	if err != nil {
		return nil, err
	}

	setLeavesReq := trillian.SetMapLeavesRequest{
		MapId:    req.MapId,
		Leaves:   req.Leaves,
		Metadata: req.Metadata,
		Revision: req.ExpectRevision}

	resp, err := t.mapServer.SetLeaves(ctx, &setLeavesReq)
	if err != nil {
		return nil, err
	}
	root, err := tcrypto.VerifySignedMapRoot(pub, hash, resp.MapRoot)
	if err != nil {
		return nil, err
	}
	return &trillian.WriteMapLeavesResponse{Revision: int64(root.Revision)}, nil
}

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianMapWriteServer) IsHealthy() error {
	return t.mapServer.IsHealthy()
}

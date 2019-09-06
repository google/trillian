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
	"errors"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/maps"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MapVerifier allows verification of output from Trillian Maps; it is safe
// for concurrent use (as its contents are fixed after construction).
type MapVerifier struct {
	*maps.RootVerifier
	MapID int64
	// RootVerifier verifies and unpacks the SMR.
	// Hasher is the hash strategy used to compute nodes in the Merkle tree.
	Hasher hashers.MapHasher
}

// NewMapVerifierFromTree creates a new MapVerifier using the information
// from a Trillian Tree object.
func NewMapVerifierFromTree(config *trillian.Tree) (*MapVerifier, error) {
	rootVerifier, err := maps.NewRootVerifierFromTree(config)
	if err != nil {
		return nil, err
	}
	return NewMapVerifier(config, rootVerifier)
}

// NewMapVerifierFromTree creates a new MapVerifier using the information
// from a Trillian Tree object.
func NewMapVerifier(config *trillian.Tree, rootVerifier *maps.RootVerifier) (*MapVerifier, error) {
	if config == nil {
		return nil, errors.New("client: NewMapVerifierFromTree(): nil config")
	}
	if got, want := config.TreeType, trillian.TreeType_MAP; got != want {
		return nil, fmt.Errorf("client: NewMapVerifierFromTree(): TreeType: %v, want %v", got, want)
	}

	mapHasher, err := hashers.NewMapHasher(config.HashStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed creating MapHasher: %w", err)
	}

	return &MapVerifier{
		RootVerifier: rootVerifier,
		MapID:        config.TreeId,
		Hasher:       mapHasher,
	}, nil
}

// VerifyMapLeafInclusion verifies a MapLeafInclusion object against a signed map root.
func (m *MapVerifier) VerifyMapLeafInclusion(smr *trillian.SignedMapRoot, leafProof *trillian.MapLeafInclusion) error {
	root, err := m.VerifySignedMapRoot(smr)
	if err != nil {
		return err
	}
	return m.VerifyMapLeafInclusionHash(root.RootHash, leafProof)
}

// VerifyMapLeafInclusionHash verifies a MapLeafInclusion object against a root hash.
func (m *MapVerifier) VerifyMapLeafInclusionHash(rootHash []byte, leafProof *trillian.MapLeafInclusion) error {
	return merkle.VerifyMapInclusionProof(m.MapID, leafProof.GetLeaf(), rootHash, leafProof.GetInclusion(), m.Hasher)
}

// VerifyMapLeavesResponse verifies the responses of GetMapLeaves and GetMapLeavesByRevision.
// To accept any map revision, pass -1 as revision.
func (m *MapVerifier) VerifyMapLeavesResponse(indexes [][]byte, revision int64, resp *trillian.GetMapLeavesResponse) ([]*trillian.MapLeaf, error) {
	if got, want := len(resp.MapLeafInclusion), len(indexes); got != want {
		return nil, status.Errorf(codes.Internal, "got %v leaves, want %v", got, want)
	}
	mapRoot, err := m.VerifySignedMapRoot(resp.GetMapRoot())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "VerifySignedMapRoot(%v): %v", m.MapID, err)
	}
	if revision != -1 && int64(mapRoot.Revision) != revision {
		return nil, status.Errorf(codes.Internal, "got map revision %v, want %v", mapRoot.Revision, revision)
	}

	var g errgroup.Group
	for _, p := range resp.MapLeafInclusion {
		p := p
		g.Go(func() error {
			return m.VerifyMapLeafInclusionHash(mapRoot.RootHash, p)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, status.Errorf(status.Code(err), "map: VerifyMapLeafInclusion(): %v", err)
	}

	leaves := make([]*trillian.MapLeaf, 0, len(resp.MapLeafInclusion))
	for _, i := range resp.MapLeafInclusion {
		leaves = append(leaves, i.Leaf)
	}
	return leaves, nil
}

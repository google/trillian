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
	"crypto"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcrypto "github.com/google/trillian/crypto"
)

// MapVerifier verifies protos produced by the Trillian Map.
type MapVerifier struct {
	MapID int64
	// Hasher is the hash strategy used to compute nodes in the Merkle tree.
	Hasher hashers.MapHasher
	// PubKey verifies the signature on the digest of MapRoot.
	PubKey crypto.PublicKey
	// SigHash computes the digest of MapRoot for signing.
	SigHash crypto.Hash
}

// NewMapVerifierFromTree creates a new MapVerifier.
func NewMapVerifierFromTree(config *trillian.Tree) (*MapVerifier, error) {
	if got, want := config.TreeType, trillian.TreeType_MAP; got != want {
		return nil, fmt.Errorf("client: NewFromTree(): TreeType: %v, want %v", got, want)
	}

	mapHasher, err := hashers.NewMapHasher(config.GetHashStrategy())
	if err != nil {
		return nil, fmt.Errorf("Failed creating MapHasher: %v", err)
	}

	mapPubKey, err := der.UnmarshalPublicKey(config.GetPublicKey().GetDer())
	if err != nil {
		return nil, fmt.Errorf("Failed parsing Map public key: %v", err)
	}

	sigHash, err := trees.Hash(config)
	if err != nil {
		return nil, fmt.Errorf("client: NewMapVerifierFromTree(): Failed parsing Map signature hash: %v", err)
	}

	return &MapVerifier{
		MapID:   config.GetTreeId(),
		Hasher:  mapHasher,
		PubKey:  mapPubKey,
		SigHash: sigHash,
	}, nil
}

// VerifyMapLeafInclusion verifies a MapLeafInclusion response against a signed map root.
func (m *MapVerifier) VerifyMapLeafInclusion(smr *trillian.SignedMapRoot, leafProof *trillian.MapLeafInclusion) error {
	root, err := m.VerifySignedMapRoot(smr)
	if err != nil {
		return err
	}
	return m.VerifyMapLeafInclusionHash(root.RootHash, leafProof)
}

// VerifyMapLeafInclusionHash verifies a MapLeafInclusion response against a root hash.
func (m *MapVerifier) VerifyMapLeafInclusionHash(rootHash []byte, leafProof *trillian.MapLeafInclusion) error {
	return merkle.VerifyMapInclusionProof(m.MapID, leafProof.GetLeaf(), rootHash, leafProof.GetInclusion(), m.Hasher)
}

// VerifySignedMapRoot verifies the signature on the SignedMapRoot.
func (m *MapVerifier) VerifySignedMapRoot(smr *trillian.SignedMapRoot) (*types.MapRootV1, error) {
	return tcrypto.VerifySignedMapRoot(m.PubKey, m.SigHash, smr)
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
	leaves := make([]*trillian.MapLeaf, 0, len(resp.MapLeafInclusion))
	for _, i := range resp.MapLeafInclusion {
		if err := m.VerifyMapLeafInclusionHash(mapRoot.RootHash, i); err != nil {
			return nil, status.Errorf(status.Code(err), "map: VerifyMapLeafInclusion(): %v", err)
		}
		leaves = append(leaves, i.Leaf)
	}
	return leaves, nil
}

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

	tcrypto "github.com/google/trillian/crypto"
)

// MapVerifier verifies protos produced by the Trillian Map.
type MapVerifier struct {
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
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): Failed parsing Log signature hash: %v", err)
	}

	return &MapVerifier{
		Hasher:  mapHasher,
		PubKey:  mapPubKey,
		SigHash: sigHash,
	}, nil
}

// VerifyMapLeafInclusion verifies a MapLeafInclusion response.
func (m *MapVerifier) VerifyMapLeafInclusion(smr *trillian.SignedMapRoot, leafProof *trillian.MapLeafInclusion) error {
	index := leafProof.GetLeaf().GetIndex()
	leaf := leafProof.GetLeaf().GetLeafValue()
	proof := leafProof.GetInclusion()
	expectedRoot := smr.GetRootHash()
	mapID := smr.GetMapId()
	return merkle.VerifyMapInclusionProof(mapID, index, leaf, expectedRoot, proof, m.Hasher)
}

// VerifySignedMapRoot verifies the signature on the SignedMapRoot.
func (m *MapVerifier) VerifySignedMapRoot(smr *trillian.SignedMapRoot) error {
	// SignedMapRoot contains its own signature. To verify, we need to create a local
	// copy of the object and return the object to the state it was in when signed
	// by removing the signature from the object.
	orig := *smr
	orig.Signature = nil // Remove the signature from the object to be verified.
	return tcrypto.VerifyObject(m.PubKey, m.SigHash, orig, smr.GetSignature())
}

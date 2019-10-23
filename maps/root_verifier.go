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

package maps

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"
)

// RootVerifier allows verification of signed root output from Trillian Maps; it
// is safe for concurrent use (as its contents are fixed after construction).
type RootVerifier struct {
	// PubKey verifies the signature on the digest of MapRoot.
	PubKey crypto.PublicKey
	// SigHash computes the digest of MapRoot for signing.
	SigHash crypto.Hash
}

// NewRootVerifierFromTree creates a new RootVerifier using the information
// from a Trillian Tree object.
func NewRootVerifierFromTree(config *trillian.Tree) (*RootVerifier, error) {
	if config == nil {
		return nil, errors.New("maps: NewRootVerifierFromTree(): nil config")
	}
	if got, want := config.TreeType, trillian.TreeType_MAP; got != want {
		return nil, fmt.Errorf("maps: NewRootVerifierFromTree(): TreeType: %v, want %v", got, want)
	}

	mapPubKey, err := der.UnmarshalPublicKey(config.PublicKey.GetDer())
	if err != nil {
		return nil, fmt.Errorf("failed parsing Map public key: %v", err)
	}

	sigHash, err := trees.Hash(config)
	if err != nil {
		return nil, fmt.Errorf("maps: NewRootVerifierFromTree(): Failed parsing Map signature hash: %v", err)
	}

	return &RootVerifier{
		PubKey:  mapPubKey,
		SigHash: sigHash,
	}, nil
}

// VerifySignedMapRoot verifies the signature on a SignedMapRoot and returns a
// verified MapRootV1 which allows access to the verified properties of the SMR.
func (m *RootVerifier) VerifySignedMapRoot(smr *trillian.SignedMapRoot) (*types.MapRootV1, error) {
	return tcrypto.VerifySignedMapRoot(m.PubKey, m.SigHash, smr)
}

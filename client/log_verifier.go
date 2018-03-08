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

package client

import (
	"crypto"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"

	tcrypto "github.com/google/trillian/crypto"
)

// LogVerifier contains state needed to verify output from Trillian Logs.
type LogVerifier struct {
	hasher hashers.LogHasher
	pubKey crypto.PublicKey
	v      merkle.LogVerifier
}

// NewLogVerifier returns an object that can verify output from Trillian Logs.
func NewLogVerifier(hasher hashers.LogHasher, pubKey crypto.PublicKey) *LogVerifier {
	return &LogVerifier{
		hasher: hasher,
		pubKey: pubKey,
		v:      merkle.NewLogVerifier(hasher),
	}
}

// NewLogVerifierFromTree creates a new LogVerifier using the algorithms
// specified by *trillian.Tree.
func NewLogVerifierFromTree(config *trillian.Tree) (*LogVerifier, error) {
	if got, want := config.TreeType, trillian.TreeType_LOG; got != want {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): TreeType: %v, want %v", got, want)
	}

	// Log Hasher.
	logHasher, err := hashers.NewLogHasher(config.GetHashStrategy())
	if err != nil {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): NewLogHasher(): %v", err)
	}

	// Log Key
	logPubKey, err := der.UnmarshalPublicKey(config.GetPublicKey().GetDer())
	if err != nil {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): Failed parsing Log public key: %v", err)
	}

	return NewLogVerifier(logHasher, logPubKey), nil
}

// VerifyRoot verifies that newRoot is a valid append-only operation from trusted.
// If trusted.TreeSize is zero, a consistency proof is not needed.
func (c *LogVerifier) VerifyRoot(trusted, newRoot *trillian.SignedLogRoot,
	consistency [][]byte) error {

	if trusted == nil {
		return fmt.Errorf("VerifyRoot() error: trusted == nil")
	}
	if newRoot == nil {
		return fmt.Errorf("VerifyRoot() error: newRoot == nil")
	}

	// Verify SignedLogRoot signature.
	if _, err := tcrypto.VerifySignedLogRoot(c.pubKey, newRoot); err != nil {
		return err
	}

	// Implicitly trust the first root we get.
	if trusted.TreeSize != 0 {
		// Verify consistency proof.
		if err := c.v.VerifyConsistencyProof(
			trusted.TreeSize, newRoot.TreeSize,
			trusted.RootHash, newRoot.RootHash,
			consistency); err != nil {
			return err
		}
	}
	return nil
}

// VerifyInclusionAtIndex verifies that the inclusion proof for data at index matches
// the currently trusted root. The inclusion proof must be requested for Root().TreeSize.
func (c *LogVerifier) VerifyInclusionAtIndex(trusted *trillian.SignedLogRoot, data []byte, leafIndex int64, proof [][]byte) error {
	if trusted == nil {
		return fmt.Errorf("VerifyInclusionAtIndex() error: trusted == nil")
	}

	leaf, err := c.BuildLeaf(data)
	if err != nil {
		return err
	}
	return c.v.VerifyInclusionProof(leafIndex, trusted.TreeSize,
		proof, trusted.RootHash, leaf.MerkleLeafHash)
}

// VerifyInclusionByHash verifies the inclusion proof for data
func (c *LogVerifier) VerifyInclusionByHash(trusted *trillian.SignedLogRoot, leafHash []byte, proof *trillian.Proof) error {
	if trusted == nil {
		return fmt.Errorf("VerifyInclusionByHash() error: trusted == nil")
	}
	if proof == nil {
		return fmt.Errorf("VerifyInclusionByHash() error: proof == nil")
	}

	return c.v.VerifyInclusionProof(proof.LeafIndex, trusted.TreeSize, proof.Hashes,
		trusted.RootHash, leafHash)
}

// BuildLeaf runs the leaf hasher over data and builds a leaf.
func (c *LogVerifier) BuildLeaf(data []byte) (*trillian.LogLeaf, error) {
	leafHash, err := c.hasher.HashLeaf(data)
	if err != nil {
		return nil, err
	}
	return &trillian.LogLeaf{
		LeafValue:      data,
		MerkleLeafHash: leafHash,
	}, nil
}

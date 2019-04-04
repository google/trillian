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
	"errors"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
)

// LogVerifier allows verification of output from Trillian Logs, both regular
// and pre-ordered; it is safe for concurrent use (as its contents are fixed
// after construction).
type LogVerifier struct {
	// Hasher is the hash strategy used to compute nodes in the Merkle tree.
	Hasher hashers.LogHasher
	// PubKey verifies the signature on the digest of LogRoot.
	PubKey crypto.PublicKey
	// SigHash computes the digest of LogRoot for signing.
	SigHash crypto.Hash
	v       merkle.LogVerifier
}

// NewLogVerifier returns an object that can verify output from Trillian Logs.
func NewLogVerifier(hasher hashers.LogHasher, pubKey crypto.PublicKey, sigHash crypto.Hash) *LogVerifier {
	return &LogVerifier{
		Hasher:  hasher,
		PubKey:  pubKey,
		SigHash: sigHash,
		v:       merkle.NewLogVerifier(hasher),
	}
}

// NewLogVerifierFromTree creates a new LogVerifier using the algorithms
// specified by a Trillian Tree object.
func NewLogVerifierFromTree(config *trillian.Tree) (*LogVerifier, error) {
	if config == nil {
		return nil, errors.New("client: NewLogVerifierFromTree(): nil config")
	}
	log, pLog := trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG
	if got := config.TreeType; got != log && got != pLog {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): TreeType: %v, want %v or %v", got, log, pLog)
	}

	logHasher, err := hashers.NewLogHasher(config.HashStrategy)
	if err != nil {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): NewLogHasher(): %v", err)
	}

	logPubKey, err := der.UnmarshalPublicKey(config.PublicKey.GetDer())
	if err != nil {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): Failed parsing Log public key: %v", err)
	}

	sigHash, err := trees.Hash(config)
	if err != nil {
		return nil, fmt.Errorf("client: NewLogVerifierFromTree(): Failed parsing Log signature hash: %v", err)
	}

	return NewLogVerifier(logHasher, logPubKey, sigHash), nil
}

// VerifyRoot verifies that newRoot is a valid append-only operation from
// trusted. If trusted.TreeSize is zero, a consistency proof is not needed.
func (c *LogVerifier) VerifyRoot(trusted *types.LogRootV1, newRoot *trillian.SignedLogRoot, consistency [][]byte) (*types.LogRootV1, error) {

	if trusted == nil {
		return nil, fmt.Errorf("VerifyRoot() error: trusted == nil")
	}
	if newRoot == nil {
		return nil, fmt.Errorf("VerifyRoot() error: newRoot == nil")
	}

	// Verify SignedLogRoot signature and unpack its contents.
	r, err := tcrypto.VerifySignedLogRoot(c.PubKey, c.SigHash, newRoot)
	if err != nil {
		return nil, err
	}

	// Implicitly trust the first root we get.
	if trusted.TreeSize != 0 {
		// Verify consistency proof.
		if err := c.v.VerifyConsistencyProof(int64(trusted.TreeSize), int64(r.TreeSize), trusted.RootHash, r.RootHash, consistency); err != nil {
			return nil, fmt.Errorf("failed to verify consistency proof from %d->%d %x->%x: %v", trusted.TreeSize, r.TreeSize, trusted.RootHash, r.RootHash, err)
		}
	}
	return r, nil
}

// VerifyInclusionAtIndex verifies that the inclusion proof for data at leafIndex
// matches the given trusted root.
func (c *LogVerifier) VerifyInclusionAtIndex(trusted *types.LogRootV1, data []byte, leafIndex int64, proof [][]byte) error {
	if trusted == nil {
		return fmt.Errorf("VerifyInclusionAtIndex() error: trusted == nil")
	}

	leaf, err := c.BuildLeaf(data)
	if err != nil {
		return err
	}
	return c.v.VerifyInclusionProof(leafIndex, int64(trusted.TreeSize),
		proof, trusted.RootHash, leaf.MerkleLeafHash)
}

// VerifyInclusionByHash verifies that the inclusion proof for the given Merkle leafHash
// matches the given trusted root.
func (c *LogVerifier) VerifyInclusionByHash(trusted *types.LogRootV1, leafHash []byte, proof *trillian.Proof) error {
	if trusted == nil {
		return fmt.Errorf("VerifyInclusionByHash() error: trusted == nil")
	}
	if proof == nil {
		return fmt.Errorf("VerifyInclusionByHash() error: proof == nil")
	}

	return c.v.VerifyInclusionProof(proof.LeafIndex, int64(trusted.TreeSize), proof.Hashes,
		trusted.RootHash, leafHash)
}

// BuildLeaf runs the leaf hasher over data and builds a leaf.
// TODO(pavelkalinnikov): This can be misleading as it creates a partially
// filled LogLeaf. Consider returning a pair instead, or leafHash only.
func (c *LogVerifier) BuildLeaf(data []byte) (*trillian.LogLeaf, error) {
	leafHash, err := c.Hasher.HashLeaf(data)
	if err != nil {
		return nil, err
	}
	return &trillian.LogLeaf{
		LeafValue:      data,
		MerkleLeafHash: leafHash,
	}, nil
}

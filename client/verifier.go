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
	"crypto/sha256"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"fmt"
)

// logVerifier contains state needed to verify output from Trillian Logs.
type logVerifier struct {
	hasher hashers.LogHasher
	pubKey crypto.PublicKey
	v      merkle.LogVerifier
}

// NewLogVerifier returns an object that can verify output from Trillian Logs.
func NewLogVerifier(hasher hashers.LogHasher, pubKey crypto.PublicKey) LogVerifier {
	return &logVerifier{
		hasher: hasher,
		pubKey: pubKey,
		v:      merkle.NewLogVerifier(hasher),
	}
}

// VerifyRoot verifies that newRoot is a valid append-only operation from trusted.
// If trusted.TreeSize is zero, a consistency proof is not needed.
func (c *logVerifier) VerifyRoot(trusted, newRoot *trillian.SignedLogRoot,
	consistency [][]byte) error {

	// Verify SignedLogRoot signature.
	if (newRoot == nil){
		return fmt.Errorf("VerifyRoot() error: newRoot == nil.")
	}else {
		hash := tcrypto.HashLogRoot(*newRoot)
		if err := tcrypto.Verify(c.pubKey, hash, newRoot.GetSignature()); err != nil {
			return err
		}
	}

	// Implicitly trust the first root we get.
	if trusted.GetTreeSize() != 0 {
		// Verify consistency proof.
		if err := c.v.VerifyConsistencyProof(
			trusted.GetTreeSize(), newRoot.GetTreeSize(),
			trusted.GetRootHash(), newRoot.GetRootHash(),
			consistency); err != nil {
			return err
		}
	}
	return nil
}

// VerifyInclusionAtIndex verifies that the inclusion proof for data at index matches
// the currently trusted root. The inclusion proof must be requested for Root().TreeSize.
func (c *logVerifier) VerifyInclusionAtIndex(trusted *trillian.SignedLogRoot, data []byte, leafIndex int64, proof [][]byte) error {
	leaf := c.buildLeaf(data)
	return c.v.VerifyInclusionProof(leafIndex, trusted.GetTreeSize(),
		proof, trusted.GetRootHash(), leaf.GetMerkleLeafHash())
}

// VerifyInclusionByHash verifies the inclusion proof for data
func (c *logVerifier) VerifyInclusionByHash(trusted *trillian.SignedLogRoot, leafHash []byte, proof *trillian.Proof) error {
	return c.v.VerifyInclusionProof(proof.GetLeafIndex(), trusted.GetTreeSize(), proof.GetHashes(),
		trusted.GetRootHash(), leafHash)
}

func (c *logVerifier) buildLeaf(data []byte) *trillian.LogLeaf {
	hash := sha256.Sum256(data)
	return &trillian.LogLeaf{
		LeafValue:        data,
		MerkleLeafHash:   c.hasher.HashLeaf(data),
		LeafIdentityHash: hash[:],
	}
}

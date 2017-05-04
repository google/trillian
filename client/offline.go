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
)

// logVerifier contains state needed to verify output from Trillian Logs.
type logVerifier struct {
	hasher merkle.TreeHasher
	pubKey crypto.PublicKey
	root   trillian.SignedLogRoot
	v      merkle.LogVerifier
}

// NewLogVerifier returns an object that can verify output from Trillian Logs.
func NewLogVerifier(hasher merkle.TreeHasher, pubKey crypto.PublicKey) LogVerifier {
	return &logVerifier{
		hasher: hasher,
		pubKey: pubKey,
		v:      merkle.NewLogVerifier(hasher),
	}
}

// Root returns the last valid root seen by UpdateRoot.
// Returns an empty SignedLogRoot if UpdateRoot has not been called.
func (c *logVerifier) Root() trillian.SignedLogRoot {
	return c.root
}

// UpdateRoot applies a GetLatestSignedLogRootResponse to Root(), if valid.
// consistency may be nil if Root().TreeSize is zero.
func (c *logVerifier) UpdateRoot(resp *trillian.GetLatestSignedLogRootResponse,
	consistency *trillian.GetConsistencyProofResponse) error {
	str := resp.SignedLogRoot

	// Verify SignedLogRoot signature.
	hash := tcrypto.HashLogRoot(*str)
	if err := tcrypto.Verify(c.pubKey, hash, str.Signature); err != nil {
		return err
	}

	// Implicitly trust the first root we get.
	if c.root.TreeSize != 0 {
		// Verify consistency proof.
		if err := c.v.VerifyConsistencyProof(
			c.root.TreeSize, str.TreeSize,
			c.root.RootHash, str.RootHash,
			consistency.GetProof().GetHashes()); err != nil {
			return err
		}
	}
	c.root = *str
	return nil
}

// VerifyInclusionAtIndex verifies that the inclusion proof for data at index matches
// the currently trusted root. The inclusion proof must be requested for Root().TreeSize.
func (c *logVerifier) VerifyInclusionAtIndex(data []byte, leafIndex int64, resp *trillian.GetInclusionProofResponse) error {
	leaf := c.buildLeaf(data)
	return c.v.VerifyInclusionProof(leafIndex, c.root.TreeSize,
		resp.Proof.Hashes, c.root.RootHash, leaf.MerkleLeafHash)

}

// VerifyInclusionByHash verifies the inclusion proof for data
func (c *logVerifier) VerifyInclusionByHash(leafHash []byte, proof *trillian.Proof) error {
	return c.v.VerifyInclusionProof(proof.LeafIndex, c.root.TreeSize, proof.Hashes,
		c.root.RootHash, leafHash)
}

func (c *logVerifier) buildLeaf(data []byte) *trillian.LogLeaf {
	hash := sha256.Sum256(data)
	return &trillian.LogLeaf{
		LeafValue:        data,
		MerkleLeafHash:   c.hasher.HashLeaf(data),
		LeafIdentityHash: hash[:],
	}
}

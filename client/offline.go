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

// LogVerifier contains state needed to verify output from Trillian Logs.
type LogVerifier struct {
	hasher merkle.TreeHasher
	pubKey crypto.PublicKey
	root   trillian.SignedLogRoot
	v      merkle.LogVerifier
}

// NewLogVerifier returns an object that can verify output from Trillian Logs.
func NewLogVerifier(hasher merkle.TreeHasher, pubKey crypto.PublicKey) *LogVerifier {
	return &LogVerifier{
		hasher: hasher,
		pubKey: pubKey,
		v:      merkle.NewLogVerifier(hasher),
	}
}

// Root returns the last valid root seen by UpdateRoot.
// Returns an empty SignedLogRoot if UpdateRoot has not been called.
func (c *LogVerifier) Root() trillian.SignedLogRoot {
	return c.root
}

// UpdateRoot applies a GetLatestSignedLogRootResponse to Root(), if valid.
func (c *LogVerifier) UpdateRoot(resp *trillian.GetLatestSignedLogRootResponse,
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
			convertProof(consistency.GetProof())); err != nil {
			return err
		}
	}
	c.root = *str
	return nil
}

// VerifyInclusionAtIndex verifies that the inclusion proof for data at index matches the currently trusted root.
func (c *LogVerifier) VerifyInclusionAtIndex(data []byte, leafIndex int64, resp *trillian.GetInclusionProofResponse) error {
	leaf := c.buildLeaf(data)
	// XXX: c.root.TreeSize used to be req.TreeSize.
	return c.v.VerifyInclusionProof(leafIndex, c.root.TreeSize,
		convertProof(resp.Proof), c.root.RootHash,
		leaf.MerkleLeafHash)

}

// VerifyInclusionByHash verifies the inclusion proof for data with Tril
func (c *LogVerifier) VerifyInclusionByHash(leafHash []byte, proof *trillian.Proof) error {
	neighbors := convertProof(proof)
	return c.v.VerifyInclusionProof(proof.LeafIndex, c.root.TreeSize, neighbors,
		c.root.RootHash, leafHash)
}

func (c *LogVerifier) buildLeaf(data []byte) *trillian.LogLeaf {
	hash := sha256.Sum256(data)
	return &trillian.LogLeaf{
		LeafValue:        data,
		MerkleLeafHash:   c.hasher.HashLeaf(data),
		LeafIdentityHash: hash[:],
	}
}

// convertProof returns a slice of neighbor nodes from a trillian Proof.
// TODO(martin): adjust the public API to do this in the server before returning a proof.
func convertProof(proof *trillian.Proof) [][]byte {
	if proof == nil {
		return [][]byte{}
	}
	neighbors := make([][]byte, len(proof.ProofNode))
	for i, node := range proof.ProofNode {
		neighbors[i] = node.NodeHash
	}
	return neighbors
}

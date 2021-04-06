// Copyright 2016 Google LLC. All Rights Reserved.
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

// Package crypto provides signing functionality for Trillian.
package crypto

import (
	"crypto"
	"crypto/rand"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"golang.org/x/crypto/ed25519"
)

const noHash = crypto.Hash(0)

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	// If Hash is noHash (zero), the signer expects to be given the full message not a hashed digest.
	Hash   crypto.Hash
	Signer crypto.Signer
}

// NewSigner returns a new signer.
func NewSigner(signer crypto.Signer, hash crypto.Hash) *Signer {
	if _, ok := signer.(ed25519.PrivateKey); ok {
		// Ed25519 signing requires the full message.
		hash = noHash
	}
	return &Signer{
		Hash:   hash,
		Signer: signer,
	}
}

// Public returns the public key that can verify signatures produced by s.
func (s *Signer) Public() crypto.PublicKey {
	return s.Signer.Public()
}

// Sign obtains a signature over the input data; this typically (but not always)
// involves first hashing the input data.
func (s *Signer) Sign(data []byte) ([]byte, error) {
	if s.Hash == noHash {
		return s.Signer.Sign(rand.Reader, data, noHash)
	}
	h := s.Hash.New()
	h.Write(data)
	digest := h.Sum(nil)

	return s.Signer.Sign(rand.Reader, digest, s.Hash)
}

// SignLogRoot returns a complete SignedLogRoot (including signature).
func (s *Signer) SignLogRoot(r *types.LogRootV1) (*trillian.SignedLogRoot, error) {
	logRoot, err := r.MarshalBinary()
	if err != nil {
		return nil, err
	}
	signature, err := s.Sign(logRoot)
	if err != nil {
		glog.Warningf("signer failed to sign log root: %v", err)
		return nil, err
	}

	return &trillian.SignedLogRoot{
		LogRoot:          logRoot,
		LogRootSignature: signature,
	}, nil
}

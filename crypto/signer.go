// Copyright 2016 Google Inc. All Rights Reserved.
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
)

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	KeyHint []byte
	Hash    crypto.Hash
	Signer  crypto.Signer
}

// NewSigner returns a new signer. The signer will set the KeyHint field, when available, with KeyID.
func NewSigner(keyID int64, signer crypto.Signer, hash crypto.Hash) *Signer {
	return &Signer{
		KeyHint: types.SerializeKeyHint(keyID),
		Hash:    hash,
		Signer:  signer,
	}
}

// NewSHA256Signer creates a new SHA256 based Signer and a KeyID of 0.
// TODO(gbelvin): remove
func NewSHA256Signer(signer crypto.Signer) *Signer {
	return &Signer{
		Hash:   crypto.SHA256,
		Signer: signer,
	}
}

// Public returns the public key that can verify signatures produced by s.
func (s *Signer) Public() crypto.PublicKey {
	return s.Signer.Public()
}

// Sign obtains a signature after first hashing the input data.
func (s *Signer) Sign(data []byte) ([]byte, error) {
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
		glog.Warningf("%v: signer failed to sign log root: %v", s.KeyHint, err)
		return nil, err
	}

	return &trillian.SignedLogRoot{
		KeyHint:          s.KeyHint,
		LogRoot:          logRoot,
		LogRootSignature: signature,
	}, nil
}

// SignMapRoot hashes and signs the supplied (to-be) SignedMapRoot and returns a signature.
func (s *Signer) SignMapRoot(r *types.MapRootV1) (*trillian.SignedMapRoot, error) {
	rootBytes, err := r.MarshalBinary()
	if err != nil {
		return nil, err
	}

	signature, err := s.Sign(rootBytes)
	if err != nil {
		glog.Warningf("%v: signer failed to sign map root: %v", s.KeyHint, err)
		return nil, err
	}

	return &trillian.SignedMapRoot{
		MapRoot:   rootBytes,
		Signature: signature,
	}, nil
}

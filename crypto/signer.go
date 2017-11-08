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
	"encoding/json"
	"fmt"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian/crypto/sigpb"
)

var sigpbHashLookup = map[crypto.Hash]sigpb.DigitallySigned_HashAlgorithm{
	crypto.SHA256: sigpb.DigitallySigned_SHA256,
}

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	Hash   crypto.Hash
	Signer crypto.Signer
}

// NewSHA256Signer creates a new SHA256 based Signer.
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
func (s *Signer) Sign(data []byte) (*sigpb.DigitallySigned, error) {
	h := s.Hash.New()
	h.Write(data)
	digest := h.Sum(nil)

	sig, err := s.Signer.Sign(rand.Reader, digest, s.Hash)
	if err != nil {
		return nil, err
	}

	return &sigpb.DigitallySigned{
		SignatureAlgorithm: SignatureAlgorithm(s.Public()),
		HashAlgorithm:      sigpbHashLookup[s.Hash],
		Signature:          sig,
	}, nil
}

// SignObject signs the requested object using ObjectHash.
func (s *Signer) SignObject(obj interface{}) (*sigpb.DigitallySigned, error) {
	// TODO(gbelvin): use objecthash.CommonJSONify
	j, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	hash, err := objecthash.CommonJSONHash(string(j))
	if err != nil {
		return nil, fmt.Errorf("CommonJSONHash(%s): %v", j, err)
	}
	return s.Sign(hash[:])
}

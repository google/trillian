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

package crypto

import (
	"crypto"
	"crypto/rand"
	"encoding/json"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian/crypto/sigpb"
)

var sigpbHashLookup = map[crypto.Hash]sigpb.DigitallySigned_HashAlgorithm{
	crypto.SHA256: sigpb.DigitallySigned_SHA256,
}

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	hash         crypto.Hash
	signer       crypto.Signer
	sigAlgorithm sigpb.DigitallySigned_SignatureAlgorithm
}

// NewSigner creates a new Signer wrapping up a hasher and a signer. For the moment
// we only support SHA256 hashing and either ECDSA or RSA signing but this is not enforced
// here.
func NewSigner(sigAlgo sigpb.DigitallySigned_SignatureAlgorithm, signer crypto.Signer) *Signer {
	return &Signer{
		hash:         crypto.SHA256,
		signer:       signer,
		sigAlgorithm: sigAlgo,
	}
}

// NewSignerFromPrivateKeyManager creates a new Signer wrapping up a hasher and a private key.
// For the moment, we only support SHA256 hashing and either ECDSA or RSA signing but this is not enforced here.
func NewSignerFromPrivateKeyManager(key PrivateKeyManager) *Signer {
	return NewSigner(key.SignatureAlgorithm(), key)
}

// Sign obtains a signature after first hashing the input data.
func (s *Signer) Sign(data []byte) (*sigpb.DigitallySigned, error) {
	h := s.hash.New()
	h.Write(data)
	digest := h.Sum(nil)

	sig, err := s.signer.Sign(rand.Reader, digest, s.hash)
	if err != nil {
		return nil, err
	}

	return &sigpb.DigitallySigned{
		SignatureAlgorithm: s.sigAlgorithm,
		HashAlgorithm:      sigpbHashLookup[s.hash],
		Signature:          sig,
	}, nil
}

// SignObject signs the requested object using ObjectHash.
func (s *Signer) SignObject(obj interface{}) (*sigpb.DigitallySigned, error) {
	j, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	hash := objecthash.CommonJSONHash(string(j))
	return s.Sign(hash[:])
}

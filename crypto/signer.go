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
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/types"
)

var sigpbHashLookup = map[crypto.Hash]sigpb.DigitallySigned_HashAlgorithm{
	crypto.SHA256: sigpb.DigitallySigned_SHA256,
}

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

// SignObject signs the requested object using ObjectHash.
func (s *Signer) SignObject(obj interface{}) ([]byte, error) {
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
		// TODO(gbelvin): Remove deprecated fields
		TimestampNanos: int64(r.TimestampNanos),
		RootHash:       r.RootHash,
		TreeSize:       int64(r.TreeSize),
		TreeRevision:   int64(r.Revision),
	}, nil
}

// SignMapRoot hashes and signs the supplied (to-be) SignedMapRoot and returns a
// signature.  Hashing is performed by github.com/benlaurie/objecthash.
func (s *Signer) SignMapRoot(root *trillian.SignedMapRoot) ([]byte, error) {
	return s.SignObject(root)
}

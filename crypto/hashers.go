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
	gocrypto "crypto"
	_ "crypto/sha256" // Register the SHA256 algorithm
	"fmt"

	"github.com/google/trillian"
)

// Hasher is the interface which must be implemented by hashers.
type Hasher struct {
	gocrypto.Hash
	alg trillian.HashAlgorithm
}

// NewHasher creates a Hasher instance for the specified algorithm.
func NewHasher(alg trillian.HashAlgorithm) (Hasher, error) {
	switch alg {
	case trillian.HashAlgorithm_SHA256:
		return Hasher{gocrypto.SHA256, alg}, nil
	}
	return Hasher{}, fmt.Errorf("unsupported hash algorithm %v", alg)
}

// Digest calculates the digest of b according to the underlying algorithm.
func (h Hasher) Digest(b []byte) []byte {
	hr := h.New()
	hr.Write(b)
	return hr.Sum([]byte{})
}

// NewSHA256 creates a Hasher instance for a stateless SHA256 hashing function.
func NewSHA256() Hasher {
	h, err := NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		// the only error we could get would be "unsupported hash algo", and since we know we support SHA256 that "can't" happen, right?
		panic(err)
	}
	return h
}

// HashAlgorithm returns an identifier for the underlying hash algorithm for a Hasher instance.
func (h Hasher) HashAlgorithm() trillian.HashAlgorithm {
	return h.alg
}

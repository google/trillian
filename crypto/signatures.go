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

package crypto

import (
	gocrypto "crypto"
	"crypto/ecdsa"
	"crypto/rsa"

	"github.com/google/trillian/crypto/sigpb"
	"golang.org/x/crypto/ed25519"
)

// SignatureAlgorithm returns the algorithm used for this public key.
// Only ECDSA and RSA keys are supported. Other key types will return sigpb.DigitallySigned_ANONYMOUS.
func SignatureAlgorithm(k gocrypto.PublicKey) sigpb.DigitallySigned_SignatureAlgorithm {
	switch k.(type) {
	case *ecdsa.PublicKey:
		return sigpb.DigitallySigned_ECDSA
	case *rsa.PublicKey:
		return sigpb.DigitallySigned_RSA
	case ed25519.PublicKey:
		return sigpb.DigitallySigned_ED25519
	}

	return sigpb.DigitallySigned_ANONYMOUS
}

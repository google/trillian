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
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/trillian"
)

// ErrVerify occurs whenever signature verification fails.
var ErrVerify = errors.New("signature verification failed")

// Verify cryptographically verifies the output of Signer.
func Verify(pub crypto.PublicKey, data []byte, sig trillian.DigitallySigned) error {
	sigAlgo := sig.SignatureAlgorithm

	// Recompute digest
	hasher, err := LookupHash(sig.HashAlgorithm)
	if err != nil {
		return err
	}
	h := hasher.New()
	h.Write(data)
	digest := h.Sum(nil)

	// Verify signature algo type
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if sigAlgo != trillian.SignatureAlgorithm_ECDSA {
			return fmt.Errorf("signature algorithm does not match public key")
		}
		return verifyECDSA(key, digest, sig.Signature)
	case *rsa.PublicKey:
		if sigAlgo != trillian.SignatureAlgorithm_RSA {
			return fmt.Errorf("signature algorithm does not match public key")
		}
		return verifyRSA(key, digest, sig.Signature, hasher, hasher)
	default:
		return fmt.Errorf("unknown private key type: %T", key)
	}
}

func verifyRSA(pub *rsa.PublicKey, hashed, sig []byte, hasher crypto.Hash, opts crypto.SignerOpts) error {
	if pssOpts, ok := opts.(*rsa.PSSOptions); ok {
		return rsa.VerifyPSS(pub, hasher, hashed, sig, pssOpts)
	}
	return rsa.VerifyPKCS1v15(pub, hasher, hashed, sig)
}

func verifyECDSA(pub *ecdsa.PublicKey, hashed, sig []byte) error {
	var ecdsaSig struct {
		R, S *big.Int
	}
	rest, err := asn1.Unmarshal(sig, &ecdsaSig)
	if err != nil {
		return ErrVerify
	}
	if len(rest) != 0 {
		return ErrVerify
	}

	if !ecdsa.Verify(pub, hashed, ecdsaSig.R, ecdsaSig.S) {
		return ErrVerify
	}
	return nil

}

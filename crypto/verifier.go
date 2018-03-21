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
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/types"
)

var (
	errVerify = errors.New("signature verification failed")

	cryptoHashLookup = map[sigpb.DigitallySigned_HashAlgorithm]crypto.Hash{
		sigpb.DigitallySigned_SHA256: crypto.SHA256,
	}
)

// VerifySignedLogRoot verifies the SignedLogRoot and returns its contents.
func VerifySignedLogRoot(pub crypto.PublicKey, hash crypto.Hash, r *trillian.SignedLogRoot) (*types.LogRootV1, error) {
	// Verify SignedLogRoot signature.
	digest, err := hashLogRoot(*r)
	if err != nil {
		return nil, err
	}
	if err := Verify(pub, hash, digest, r.Signature); err != nil {
		return nil, err
	}
	return &types.LogRootV1{
		TreeSize:       uint64(r.TreeSize),
		RootHash:       r.RootHash,
		TimestampNanos: uint64(r.TimestampNanos),
		Revision:       uint64(r.TreeRevision),
	}, nil

}

// VerifySignedMapRoot verifies the signature on the SignedMapRoot.
// VerifySignedMapRoot returns MapRootV1 to encourage safe API use.
// It should be the only function available to clients that returns MapRootV1.
func VerifySignedMapRoot(pub crypto.PublicKey, hash crypto.Hash, smr *trillian.SignedMapRoot) (*types.MapRootV1, error) {
	if err := Verify(pub, hash, smr.MapRoot, smr.Signature); err != nil {
		return nil, err
	}
	var root types.MapRootV1
	if err := root.UnmarshalBinary(smr.MapRoot); err != nil {
		return nil, err
	}
	return &root, nil
}

// Verify cryptographically verifies the output of Signer.
func Verify(pub crypto.PublicKey, hash crypto.Hash, data []byte, sig *sigpb.DigitallySigned) error {
	if sig == nil {
		return errors.New("signature is nil")
	}

	if got, want := sig.SignatureAlgorithm, SignatureAlgorithm(pub); got != want {
		return fmt.Errorf("signature algorithm does not match public key, got:%v, want:%v", got, want)
	}

	// Recompute digest
	hasher, ok := cryptoHashLookup[sig.HashAlgorithm]
	if !ok {
		return fmt.Errorf("unsupported hash algorithm %v", hasher)
	}

	if hasher != hash {
		return fmt.Errorf("hash algorithm in sig %v, want %v", hasher, hash)
	}

	h := hasher.New()
	h.Write(data)
	digest := h.Sum(nil)

	switch pub := pub.(type) {
	case *ecdsa.PublicKey:
		return verifyECDSA(pub, digest, sig.Signature)
	case *rsa.PublicKey:
		return verifyRSA(pub, digest, sig.Signature, hasher, hasher)
	default:
		return fmt.Errorf("unknown private key type: %T", pub)
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
		return errVerify
	}
	if len(rest) != 0 {
		return errVerify
	}

	if !ecdsa.Verify(pub, hashed, ecdsaSig.R, ecdsaSig.S) {
		return errVerify
	}
	return nil

}

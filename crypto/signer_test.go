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
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

const message string = "testing"

func TestSign(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSigner(0, key, crypto.SHA256)

	for _, test := range []struct {
		message []byte
	}{
		{message: []byte("message")},
	} {
		sig, err := signer.Sign(test.message)
		if err != nil {
			t.Errorf("Failed to sign message: %v", err)
			continue
		}
		if got := len(sig.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := sig.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %s expected %s", got, want)
		}
		if got, want := sig.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %s expected %s", got, want)
		}
		// Check that the signature is correct
		if err := Verify(key.Public(), crypto.SHA256, test.message, sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.message, err)
		}
	}
}

func TestSign_SignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load private key: %v", err)
	}

	_, err = NewSigner(0, testonly.NewSignerWithErr(key, errors.New("sign")), crypto.SHA256).Sign([]byte(message))
	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	testonly.EnsureErrorContains(t, err, "sign")
}

func TestSignWithSignedLogRoot_SignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	s := testonly.NewSignerWithErr(key, errors.New("signfail"))
	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	hash, err := hashLogRoot(root)
	if err != nil {
		t.Fatalf("HashLogRoot(): %v", err)
	}
	_, err = NewSigner(0, s, crypto.SHA256).Sign(hash)
	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSigner(0, key, crypto.SHA256)

	for _, test := range []struct {
		root *types.LogRootV1
	}{
		{root: &types.LogRootV1{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}},
	} {
		slr, err := signer.SignLogRoot(test.root)
		if err != nil {
			t.Errorf("Failed to sign log root: %v", err)
			continue
		}
		if got := len(slr.Signature.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := slr.Signature.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %s expected %s", got, want)
		}
		if got, want := slr.Signature.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %s expected %s", got, want)
		}
		// Check that the signature is correct
		if _, err := VerifySignedLogRoot(key.Public(), crypto.SHA256, slr); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

func TestSignMapRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSigner(0, key, crypto.SHA256)

	for _, root := range []types.MapRootV1{
		{TimestampNanos: 2267709, RootHash: []byte("Islington"), Revision: 3},
	} {
		smr, err := signer.SignMapRoot(&root)
		if err != nil {
			t.Errorf("Failed to sign map root: %v", err)
			continue
		}
		if got := len(smr.Signature.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := smr.Signature.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %s expected %s", got, want)
		}
		if got, want := smr.Signature.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %s expected %s", got, want)
		}

		if err := VerifySignedMapRoot(key.Public(), smr); err != nil {
			t.Errorf("Verify(%v) failed: %v", root, err)
		}
	}
}

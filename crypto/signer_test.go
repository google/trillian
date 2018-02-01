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
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
)

const message = "testing"

var rootHash = make([]byte, 32)

func TestSign(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

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
		if err := Verify(key.Public(), test.message, sig); err != nil {
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

	_, err = NewSHA256Signer(testonly.NewSignerWithErr(key, errors.New("sign"))).Sign([]byte(message))
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
	root := &trillian.SignedLogRoot{
		TimestampNanos: 2267709,
		RootHash:       rootHash,
		TreeSize:       2,
	}
	hash, err := CanonicalLogRoot(root, LogRootV0)
	if err != nil {
		t.Fatalf("CanonicalLogRoot(): %v", err)
	}
	_, err = NewSHA256Signer(s).Sign(hash)
	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	for _, test := range []struct {
		root *trillian.SignedLogRoot
	}{
		{root: &trillian.SignedLogRoot{
			TimestampNanos: 2267709,
			RootHash:       rootHash,
			TreeSize:       2,
		}},
	} {
		sig, err := signer.SignLogRoot(test.root)
		if err != nil {
			t.Errorf("Failed to sign log root: %v", err)
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
		obj, err := CanonicalLogRoot(test.root, LogRootV0)
		if err != nil {
			t.Errorf("CanonicalLogRoot err: got %v want nil", err)
			continue
		}

		if err := Verify(key.Public(), obj, sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

func TestSignMapRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	for _, test := range []struct {
		root *trillian.SignedMapRoot
	}{
		{root: &trillian.SignedMapRoot{
			TimestampNanos: 2267709,
			RootHash:       rootHash,
			MapRevision:    3,
		}},
	} {
		sig, err := signer.SignMapRoot(test.root)
		if err != nil {
			t.Errorf("Failed to sign map root: %v", err)
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
		canonical, err := CanonicalMapRoot(test.root, MapRootV0)
		if err != nil {
			t.Errorf("CanonicalMapRoot(): %v", err)
		}
		if err := Verify(key.Public(), canonical, sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

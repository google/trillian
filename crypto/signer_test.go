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
	"encoding/json"
	"errors"
	"testing"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
)

const message string = "testing"

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
		}
		if got := len(sig.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := sig.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %v expected %d", got, want)
		}
		if got, want := sig.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %v expected %v", got, want)
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
	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	hash, err := HashLogRoot(root)
	if err != nil {
		t.Fatalf("HashLogRoot(): %v", err)
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
		root trillian.SignedLogRoot
	}{
		{root: trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}},
	} {
		sig, err := signer.SignLogRoot(&test.root)
		if err != nil {
			t.Errorf("Failed to sign log root: %v", err)
		}
		if got := len(sig.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := sig.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %v expected %d", got, want)
		}
		if got, want := sig.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %v expected %v", got, want)
		}
		// Check that the signature is correct
		obj, err := HashLogRoot(test.root)
		if err != nil {
			t.Errorf("HashLogRoot err: got %v want nil", err)
		}

		if err := Verify(key.Public(), obj, sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

func TestSignLogRoot_SignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	s := testonly.NewSignerWithErr(key, errors.New("signfail"))
	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err = NewSHA256Signer(s).SignLogRoot(&root)
	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignMapRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	for _, test := range []struct {
		root trillian.SignedMapRoot
	}{
		{root: trillian.SignedMapRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), MapRevision: 3}},
	} {
		sig, err := signer.SignMapRoot(&test.root)
		if err != nil {
			t.Errorf("Failed to sign map root: %v", err)
		}
		if got := len(sig.Signature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		if got, want := sig.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
			t.Errorf("Hash alg incorrect, got %v expected %d", got, want)
		}
		if got, want := sig.SignatureAlgorithm, sigpb.DigitallySigned_ECDSA; got != want {
			t.Errorf("Sig alg incorrect, got %v expected %v", got, want)
		}
		// Check that the signature is correct
		j, err := json.Marshal(test.root)
		if err != nil {
			t.Errorf("json.Marshal err: %v want nil", err)
		}
		hash, err := objecthash.CommonJSONHash(string(j))
		if err != nil {
			t.Errorf("objecthash.CommonJSONHash err: %v want nil", err)
		}
		if err := Verify(key.Public(), hash[:], sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

func TestSignMapRoot_SignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	s := testonly.NewSignerWithErr(key, errors.New("signfail"))
	root := trillian.SignedMapRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), MapRevision: 3}
	_, err = NewSHA256Signer(s).SignMapRoot(&root)
	testonly.EnsureErrorContains(t, err, "signfail")
}

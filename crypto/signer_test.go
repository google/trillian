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
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
)

const message string = "testing"
const result string = "echo"

func messageHash() []byte {
	h := sha256.New()
	h.Write([]byte(message))
	return h.Sum(nil)
}

type usesSHA256Hasher struct{}

func (i usesSHA256Hasher) Matches(x interface{}) bool {
	h, ok := x.(crypto.SignerOpts)
	if !ok {
		return false
	}
	return h.HashFunc() == crypto.SHA256
}

func (i usesSHA256Hasher) String() string {
	return "uses SHA256 hasher"
}

func TestSigner(t *testing.T) {
	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key")
	}
	signer := NewSigner(key)

	pk, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		t.Fatalf("Failed to load public key")
	}

	for _, test := range []struct {
		message []byte
	}{
		{message: []byte("message")},
	} {
		sig, err := signer.Sign(test.message)
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
		if err := Verify(pk, test.message, sig); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.message, err)
		}
	}
}

func TestSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load private key: %v", err)
	}

	_, err = NewSigner(testonly.NewSignerWithErr(key, errors.New("sign"))).Sign([]byte(message))
	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	testonly.EnsureErrorContains(t, err, "sign")
}

func TestSignLogRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	s := testonly.NewSignerWithErr(key, errors.New("signfail"))
	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err = NewSigner(s).Sign(HashLogRoot(root))

	testonly.EnsureErrorContains(t, err, "signfail")
}

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
	km, err := NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key")
	}
	signer := NewSignerFromPrivateKeyManager(km)

	pk, err := PublicKeyFromPEM(testonly.DemoPublicKey)
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

	mockKey := NewMockPrivateKeyManager(ctrl)
	digest := messageHash()
	mockKey.EXPECT().Sign(gomock.Any(), digest[:], usesSHA256Hasher{}).Return(digest, errors.New("sign"))

	logSigner := NewSigner(sigpb.DigitallySigned_ECDSA, mockKey)

	_, err := logSigner.Sign([]byte(message))

	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	testonly.EnsureErrorContains(t, err, "sign")
}

func TestSignLogRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKey := NewMockPrivateKeyManager(ctrl)
	mockKey.EXPECT().Sign(gomock.Any(),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		usesSHA256Hasher{}).Return([]byte{}, errors.New("signfail"))

	logSigner := NewSigner(sigpb.DigitallySigned_ECDSA, mockKey)

	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err := logSigner.Sign(HashLogRoot(root))

	testonly.EnsureErrorContains(t, err, "signfail")
}

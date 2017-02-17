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
	"bytes"
	"crypto"
	"crypto/sha256"
	"errors"
	"reflect"
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)
	digest := messageHash()

	mockSigner.EXPECT().Sign(gomock.Any(), digest, usesSHA256Hasher{}).Return([]byte(result), nil)

	logSigner := createTestSigner(mockSigner)

	sig, err := logSigner.Sign([]byte(message))

	if err != nil {
		t.Fatalf("Failed to sign: %s", err)
	}

	if got, want := sig.HashAlgorithm, sigpb.DigitallySigned_SHA256; got != want {
		t.Fatalf("Hash alg incorrect, got %v expected %d", got, want)
	}
	if got, want := sig.SignatureAlgorithm, sigpb.DigitallySigned_RSA; got != want {
		t.Fatalf("Sig alg incorrect, got %v expected %v", got, want)
	}
	if got, want := []byte(result), sig.Signature; !bytes.Equal(got, want) {
		t.Fatalf("Mismatched sig got [%v] expected [%v]", got, want)
	}
}

func TestSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)
	digest := messageHash()

	mockSigner.EXPECT().Sign(gomock.Any(), digest[:], usesSHA256Hasher{}).Return(digest, errors.New("sign"))

	logSigner := createTestSigner(mockSigner)

	_, err := logSigner.Sign([]byte(message))

	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	testonly.EnsureErrorContains(t, err, "sign")
}

func TestSignLogRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)

	mockSigner.EXPECT().Sign(gomock.Any(),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		usesSHA256Hasher{}).Return([]byte{}, errors.New("signfail"))

	logSigner := createTestSigner(mockSigner)

	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err := logSigner.SignLogRoot(root)

	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)

	mockSigner.EXPECT().Sign(gomock.Any(),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		usesSHA256Hasher{}).Return([]byte(result), nil)

	logSigner := createTestSigner(mockSigner)

	root := trillian.SignedLogRoot{
		TimestampNanos: 2267709,
		RootHash:       []byte("Islington"),
		TreeSize:       2,
	}
	signature, err := logSigner.SignLogRoot(root)

	if err != nil {
		t.Fatalf("Failed to sign log root: %v", err)
	}

	// Check root is not modified
	expected := trillian.SignedLogRoot{
		TimestampNanos: 2267709,
		RootHash:       []byte("Islington"), TreeSize: 2}
	if !reflect.DeepEqual(root, expected) {
		t.Fatalf("Got %v, but expected unmodified signed root %v", root, expected)
	}
	// And signature is correct
	expectedSignature := sigpb.DigitallySigned{
		SignatureAlgorithm: sigpb.DigitallySigned_RSA,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		Signature:          []byte("echo")}
	if !reflect.DeepEqual(*signature, expectedSignature) {
		t.Fatalf("Got %v, but expected %v", signature, expectedSignature)
	}
}

func createTestSigner(mock *MockSigner) *Signer {
	return NewSigner(sigpb.DigitallySigned_RSA, mock)
}

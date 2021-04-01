// Copyright 2017 Google LLC. All Rights Reserved.
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

package client

import (
	"crypto"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/pem"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
)

func TestVerifyRootErrors(t *testing.T) {
	// Test setup
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := tcrypto.NewSigner(key, crypto.SHA256)
	pk, err := pem.UnmarshalPublicKey(testonly.DemoPublicKey)
	if err != nil {
		t.Fatalf("Failed to load public key, err=%v", err)
	}

	signedRoot, err := signer.SignLogRoot(&types.LogRootV1{})
	if err != nil {
		t.Fatal("Failed to create test signature")
	}

	// Test execution
	tests := []struct {
		desc    string
		trusted *types.LogRootV1
		newRoot *trillian.SignedLogRoot
	}{
		{desc: "newRootNil", trusted: &types.LogRootV1{}, newRoot: nil},
		{desc: "trustedNil", trusted: nil, newRoot: signedRoot},
	}
	for _, test := range tests {
		logVerifier := NewLogVerifier(rfc6962.DefaultHasher, pk, crypto.SHA256)

		// This also makes sure that no nil pointer dereference errors occur (as this would cause a panic).
		if _, err := logVerifier.VerifyRoot(test.trusted, test.newRoot, nil); err == nil {
			t.Errorf("%v: VerifyRoot() error expected, but got nil", test.desc)
		}
	}
}

func TestVerifyInclusionAtIndexErrors(t *testing.T) {
	logVerifier := NewLogVerifier(nil, nil, crypto.SHA256)
	// An error is expected because the first parameter (trusted) is nil
	err := logVerifier.VerifyInclusionAtIndex(nil, []byte{0, 0, 0}, 1, [][]byte{{0, 0}})
	if err == nil {
		t.Errorf("VerifyInclusionAtIndex() error expected, but got nil")
	}
}

func TestVerifyInclusionByHashErrors(t *testing.T) {
	tests := []struct {
		desc    string
		trusted *types.LogRootV1
		proof   *trillian.Proof
	}{
		{desc: "trustedNil", trusted: nil, proof: &trillian.Proof{}},
		{desc: "proofNil", trusted: &types.LogRootV1{}, proof: nil},
	}
	for _, test := range tests {

		logVerifier := NewLogVerifier(nil, nil, crypto.SHA256)
		err := logVerifier.VerifyInclusionByHash(test.trusted, nil, test.proof)
		if err == nil {
			t.Errorf("%v: VerifyInclusionByHash() error expected, but got nil", test.desc)
		}
	}
}

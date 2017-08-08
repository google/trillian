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

package client

import (
	"testing"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/testonly"
	"strings"
)

func TestVerifyRootErrors(t *testing.T) {
	// Test setup
	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := tcrypto.NewSHA256Signer(key)
	pk, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		t.Fatalf("Failed to load public key, err=%v", err)
	}

	signedRoot := trillian.SignedLogRoot{0, nil, 0, nil, 0, 0}
	hash := tcrypto.HashLogRoot(signedRoot)
	signature, err := signer.Sign(hash)
	if err != nil {
		t.Fatal("Failed to create test signature")
	}
	signedRoot.Signature = signature

	// Test execution
	tests := []struct {
		desc             string
		trusted, newRoot *trillian.SignedLogRoot
	}{
		{desc: "newRootNil", trusted: &signedRoot, newRoot: nil},
		{desc: "trustedNil", trusted: nil, newRoot: &signedRoot},
	}
	for _, test := range tests {
		logVerifier := NewLogVerifier(rfc6962.DefaultHasher, pk)

		// This also makes sure that no nil pointer dereference errors occur (as this would cause a panic).
		if err := logVerifier.VerifyRoot(test.trusted, test.newRoot, nil); err == nil {
			t.Errorf("%v: VerifyRoot() error expected, but got nil", test.desc)
		}
	}
}

func TestVerifyInclusionAtIndexErrors(t *testing.T) {
	logVerifier := NewLogVerifier(nil, nil)
	err := logVerifier.VerifyInclusionAtIndex(nil, nil, 1, nil)
	if err == nil {
		t.Errorf("VerifyInclusionAtIndex() error expected, but none was thrown")
	}
	if !strings.Contains(err.Error(), "trusted == nil"){
		t.Errorf("VerifyInclusionAtIndex() throws wrong error when trusted == nil: %v", err)
	}
}

func TestVerifyInclusionByHashErrors(t *testing.T) {
	tests := []struct {
		desc             string
		trusted *trillian.SignedLogRoot
		proof *trillian.Proof
		errorDesc string
	}{
		{desc: "trustedNil", trusted: nil, proof: &trillian.Proof{}, errorDesc: "trusted == nil"},
		{desc: "proofNil", trusted: &trillian.SignedLogRoot{}, proof: nil, errorDesc: "proof == nil"},
	}
	for _, test := range tests {

		logVerifier := NewLogVerifier(nil, nil)
		err := logVerifier.VerifyInclusionByHash(test.trusted, nil, test.proof)
		if err == nil {
			t.Errorf("%v: VerifyInclusionByHash() error expected, but none was thrown", test.desc)
		}
		if !strings.Contains(err.Error(), test.errorDesc){
			t.Errorf("%v: VerifyInclusionByHash() error expected to contain %v, but got: %v", test.desc, test.errorDesc, err)
		}
	}

}
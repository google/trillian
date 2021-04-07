// Copyright 2016 Google LLC. All Rights Reserved.
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
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

const message string = "testing"

func TestSign(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSigner(key, crypto.SHA256)

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
		if got := len(sig); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
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

	_, err = NewSigner(testonly.NewSignerWithErr(key, errors.New("sign")), crypto.SHA256).Sign([]byte(message))
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
	root := &types.LogRootV1{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err = NewSigner(s, crypto.SHA256).SignLogRoot(root)
	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSigner(key, crypto.SHA256)

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
		if got := len(slr.LogRootSignature); got == 0 {
			t.Errorf("len(sig): %v, want > 0", got)
		}
		// Check that the signature is correct
		if err := Verify(key.Public(), crypto.SHA256, slr.LogRoot, slr.LogRootSignature); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

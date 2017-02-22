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
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
)

func TestSignLogRoot(t *testing.T) {
	km, err := NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key")
	}
	signer := NewSigner(km.SignatureAlgorithm(), km)
	pk, err := PublicKeyFromPEM(testonly.DemoPublicKey)
	if err != nil {
		t.Fatalf("Failed to load public key")
	}

	for _, test := range []struct {
		root trillian.SignedLogRoot
	}{
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Islington"),
				TreeSize:       2,
			},
		},
	} {
		signature, err := signer.SignLogRoot(test.root)
		if err != nil {
			t.Errorf("Failed to sign log root: %v", err)
		}
		// Check that the signature is correct
		h := HashLogRoot(test.root)
		if err := Verify(pk, h, signature); err != nil {
			t.Errorf("Verify(%v) failed: %v", test.root, err)
		}
	}
}

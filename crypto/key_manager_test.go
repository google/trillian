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
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/google/trillian/testonly"
)

type ecdsaSig struct {
	R, S *big.Int
}

func TestLoadDemoECDSAKeyAndSign(t *testing.T) {
	// Obviously in real code we wouldn't use a fixed seed
	randSource := rand.New(rand.NewSource(42))
	hasher := crypto.SHA256

	km, err := NewFromPrivatePEM([]byte(testonly.DemoPrivateKey), testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to load key: %v", err)
	}

	signed, err := km.Signer().Sign(randSource, []byte("hello"), hasher)

	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	// Do a round trip by verifying the signature using the public key
	var signature ecdsaSig
	_, err = asn1.Unmarshal(signed, &signature)

	if err != nil {
		t.Fatal("Failed to unmarshal signature as asn.1")
	}

	publicBlock, rest := pem.Decode([]byte(testonly.DemoPublicKey))
	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)

	if len(rest) > 0 {
		t.Fatal("Extra data after key in PEM string")
	}

	if err != nil {
		panic(fmt.Errorf("Public test key failed to parse: %v", err))
	}

	publicKey := parsedKey.(*ecdsa.PublicKey)

	if !ecdsa.Verify(publicKey, []byte("hello"), signature.R, signature.S) {
		t.Fatal("Signature did not verify on round trip test")
	}
}

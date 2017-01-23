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
	km := new(PEMKeyManager)

	// Obviously in real code we wouldn't use a fixed seed
	randSource := rand.New(rand.NewSource(42))

	hasher := NewSHA256()

	err := km.LoadPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)

	if err != nil {
		t.Fatalf("Failed to load key: %v", err)
	}

	signer, err := km.Signer()

	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	signed, err := signer.Sign(randSource, []byte("hello"), hasher)

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

func TestLoadDemoECDSAPublicKey(t *testing.T) {
	km := new(PEMKeyManager)

	if err := km.LoadPublicKey(testonly.DemoPublicKey); err != nil {
		t.Fatal("Failed to load public key")
	}

	key, err := km.GetPublicKey()
	if err != nil {
		t.Fatalf("Unexpected error getting public key: %v", err)
	}

	if key == nil {
		t.Fatal("Key manager did not return public key after loading it")
	}

	// Additional sanity check on type as we know it must be an ECDSA key

	if _, ok := key.(*ecdsa.PublicKey); !ok {
		t.Fatalf("Expected to have loaded an ECDSA key but got: %v", key)
	}
}

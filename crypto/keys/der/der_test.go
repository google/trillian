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

package der_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	. "github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keys/testonly"
	"github.com/google/trillian/crypto/keyspb"
)

const (
	// ECDSA private key in DER format, base64-encoded.
	privKeyBase64 = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgS81mfpvtTmaINn+gtrYXn4XpxxgE655GLSKsA3hhjHmhRANCAASwBWDdgHS04V/cN0LZgc8vZaK4I1HWLLCoaOO27Z0B1aS1aqBE7g1Oo8ldSCBJAvee866kcHhZkVniPdCG2ZZG"
	// ECDSA public key in DER format, base64-encoded.
	pubKeyBase64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEsAVg3YB0tOFf3DdC2YHPL2WiuCNR1iywqGjjtu2dAdWktWqgRO4NTqPJXUggSQL3nvOupHB4WZFZ4j3QhtmWRg=="
)

func TestFromProto(t *testing.T) {
	t.Parallel()

	keyDER, err := base64.StdEncoding.DecodeString(privKeyBase64)
	if err != nil {
		t.Fatalf("Could not decode test key: %v", err)
	}

	for _, test := range []struct {
		desc     string
		keyProto *keyspb.PrivateKey
		wantErr  bool
	}{
		{
			desc: "PrivateKey",
			keyProto: &keyspb.PrivateKey{
				Der: keyDER,
			},
		},
		{
			desc: "PrivateKey with invalid DER",
			keyProto: &keyspb.PrivateKey{
				Der: []byte("foobar"),
			},
			wantErr: true,
		},
		{
			desc:     "PrivateKey with missing DER",
			keyProto: &keyspb.PrivateKey{},
			wantErr:  true,
		},
	} {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			signer, err := FromProto(test.keyProto)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("FromProto(%#v) = (_, %q), want (_, nil)", test.keyProto, err)
			} else if gotErr {
				return
			}

			// Check that the returned signer can produce signatures successfully.
			if err := testonly.SignAndVerify(signer, signer.Public()); err != nil {
				t.Fatalf("SignAndVerify() = %q, want nil", err)
			}
		})
	}
}

func TestMarshalUnmarshalPublicKey(t *testing.T) {
	t.Parallel()

	keyDER, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		t.Fatalf("Could not decode test key: %v", err)
	}

	key, err := UnmarshalPublicKey(keyDER)
	if err != nil {
		t.Fatalf("UnmarshalPublicKey(%v): %v", keyDER, err)
	}

	keyDER2, err := MarshalPublicKey(key)
	if err != nil {
		t.Fatalf("MarshalPublicKey(%v): %v", key, err)
	}

	if got, want := keyDER2, keyDER; !bytes.Equal(got, want) {
		t.Errorf("MarshalPublicKey(): %x, want %x", got, want)
	}
}

func TestFromToPublicProto(t *testing.T) {
	t.Parallel()

	keyDER, err := base64.StdEncoding.DecodeString(pubKeyBase64)
	if err != nil {
		t.Fatalf("Could not decode test key: %v", err)
	}

	key, err := UnmarshalPublicKey(keyDER)
	if err != nil {
		t.Fatalf("UnmarshalPublicKey(%v): %v", keyDER, err)
	}

	keyProto, err := ToPublicProto(key)
	if err != nil {
		t.Fatalf("ToPublicProto(%v): %v", key, err)
	}

	key2, err := FromPublicProto(keyProto)
	if err != nil {
		t.Fatalf("FromPublicProto(%v): %v", keyProto, err)
	}

	keyDER2, err := MarshalPublicKey(key2)
	if err != nil {
		t.Fatalf("MarshalPublicKey(%v): %v", key2, err)
	}

	if got, want := keyDER2, keyDER; !bytes.Equal(got, want) {
		t.Errorf("MarshalPublicKey(): %x, want %x", got, want)
	}
}

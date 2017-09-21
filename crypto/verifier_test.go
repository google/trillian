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
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/google/trillian/testonly"
)

const (
	// openssl ecparam -name prime256v1 -genkey -out p256-key.pem
	privPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIGbhE2+z8d5lHzb0gmkS78d86gm5gHUtXCpXveFbK3pcoAoGCCqGSM49
AwEHoUQDQgAEUxX42oxJ5voiNfbjoz8UgsGqh1bD1NXK9m8VivPmQSoYUdVFgNav
csFaQhohkiCEthY51Ga6Xa+ggn+eTZtf9Q==
-----END EC PRIVATE KEY-----`
)

func TestSignVerify(t *testing.T) {
	for _, test := range []struct {
		name          string
		pem           string
		password      string
		skipSigning   bool
		wantVerifyErr bool
	}{
		{
			name:     "ECDSA key",
			pem:      privPEM,
			password: "",
		},
		{
			name:     "Demo key",
			pem:      testonly.DemoPrivateKey,
			password: testonly.DemoPrivateKeyPass,
		},
		{
			name:          "Nil signature",
			pem:           testonly.DemoPrivateKey,
			password:      testonly.DemoPrivateKeyPass,
			skipSigning:   true,
			wantVerifyErr: true,
		},
	} {

		key, err := pem.UnmarshalPrivateKey(test.pem, test.password)
		if err != nil {
			t.Errorf("%s: LoadPrivateKey(_, %q)=%v, want nil", test.name, test.password, err)
			continue
		}

		// Sign and Verify.
		msg := []byte("foo")
		var signature *sigpb.DigitallySigned
		if !test.skipSigning {
			signature, err = NewSHA256Signer(key).Sign(msg)
			if err != nil {
				t.Errorf("%s: Sign()=(_,%v), want (_,nil)", test.name, err)
				continue
			}
		}

		err = Verify(key.Public(), msg, signature)
		if gotErr := err != nil; gotErr != test.wantVerifyErr {
			t.Errorf("%s: Verify(,,)=%v, want err? %t", test.name, err, test.wantVerifyErr)
		}
	}
}

func TestSignVerifyObject(t *testing.T) {
	key, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	type subfield struct {
		c int
	}

	meta := testonly.MustMarshalAny(t, &ctmapperpb.MapperMetadata{})
	meta0 := testonly.MustMarshalAny(t, &ctmapperpb.MapperMetadata{HighestFullyCompletedSeq: 0})
	meta1 := testonly.MustMarshalAny(t, &ctmapperpb.MapperMetadata{HighestFullyCompletedSeq: 1})

	for _, tc := range []struct {
		obj interface{}
	}{
		{meta},
		{meta0},
		{meta1},

		{&trillian.SignedMapRoot{}},
		{&trillian.SignedMapRoot{
			MapId: 0xcafe,
		}},
		{&trillian.SignedMapRoot{Metadata: meta}},
		{&trillian.SignedMapRoot{Metadata: meta0}},
		{&trillian.SignedMapRoot{Metadata: meta1}},
		{struct{ a string }{a: "foo"}},
		{struct {
			a int
			b *subfield
		}{a: 1, b: &subfield{c: 0}}},
		{struct {
			a int
			b *subfield
		}{a: 1, b: nil}},
	} {
		sig, err := signer.SignObject(tc.obj)
		if err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
			continue
		}
		if err := VerifyObject(key.Public(), tc.obj, sig); err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
		}
	}
}

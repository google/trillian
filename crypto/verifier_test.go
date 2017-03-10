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
	"crypto"
	"testing"

	"github.com/google/trillian/crypto/keys"
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
		PEM      string
		password string
		HashAlgo crypto.Hash
	}{
		{privPEM, "", crypto.SHA256},
		{testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass, crypto.SHA256},
	} {

		key, err := keys.NewFromPrivatePEM(test.PEM, test.password)
		if err != nil {
			t.Errorf("LoadPrivateKey(_, %q)=%v, want nil", test.password, err)
			continue
		}

		// Sign and Verify.
		msg := []byte("foo")
		signed, err := NewSigner(key).Sign(msg)
		if err != nil {
			t.Errorf("Sign()=(_,%v), want (_,nil)", err)
			continue
		}

		if err := Verify(key.Public(), msg, signed); err != nil {
			t.Errorf("Verify(,,)=%v, want nil", err)
		}
	}
}

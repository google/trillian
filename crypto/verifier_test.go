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
	"testing"

	"github.com/google/trillian"
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
	for _, tc := range []struct {
		PEM      string
		HashAlgo trillian.HashAlgorithm
		SigAlgo  trillian.SignatureAlgorithm
	}{
		{privPEM, trillian.HashAlgorithm_SHA256, trillian.SignatureAlgorithm_ECDSA},
	} {

		km := NewPEMKeyManager()
		if err := km.LoadPrivateKey(tc.PEM, ""); err != nil {
			t.Error(err)
			continue
		}
		kmsigner, err := km.Signer()
		if err != nil {
			t.Error(err)
			continue
		}
		hasher, err := NewHasher(tc.HashAlgo)
		if err != nil {
			t.Error(err)
			continue
		}
		signer := NewSigner(hasher, tc.SigAlgo, kmsigner)

		// Sign and Verify.
		msg := []byte("foo")
		signed, err := signer.Sign(msg)
		if err != nil {
			t.Error(err)
			continue
		}
		pub, err := km.GetPublicKey()
		if err != nil {
			t.Error(err)
			continue
		}
		if err := Verify(pub, msg, signed); err != nil {
			t.Error(err)
		}
	}
}

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

	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
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

		key, err := keys.NewFromPrivatePEM(test.pem, test.password)
		if err != nil {
			t.Errorf("%s: LoadPrivateKey(_, %q)=%v, want nil", test.name, test.password, err)
			continue
		}

		// Sign and Verify.
		msg := []byte("foo")
		var signature *sigpb.DigitallySigned
		if !test.skipSigning {
			signature, err = NewSigner(key).Sign(msg)
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

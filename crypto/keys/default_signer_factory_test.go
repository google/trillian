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

package keys

import (
	"testing"

	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keyspb"
)

func TestDefaultSignerFactory(t *testing.T) {
	key, err := pem.NewFromPrivatePEMFile("../../testdata/log-rpc-server.privkey.pem", "towel")
	if err != nil {
		t.Fatalf("Failed to load private key: %v", err)
	}

	keyDER, err := der.MarshalPrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal private key to DER: %v", err)
	}

	tester := SignerFactoryTester{
		NewSignerFactory: func() SignerFactory { return &DefaultSignerFactory{} },
		NewSignerTests: []NewSignerTest{
			{
				Name: "PEMKeyFile",
				KeyProto: &keyspb.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "towel",
				},
			},
			{
				Name: "PemKeyFile with non-existent file",
				KeyProto: &keyspb.PEMKeyFile{
					Path: "non-existent.pem",
				},
				WantErr: true,
			},
			{
				Name: "PemKeyFile with wrong password",
				KeyProto: &keyspb.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "wrong-password",
				},
				WantErr: true,
			},
			{
				Name: "PemKeyFile with missing password",
				KeyProto: &keyspb.PEMKeyFile{
					Path: "../../testdata/log-rpc-server.privkey.pem",
				},
				WantErr: true,
			},
			{
				Name: "PrivateKey",
				KeyProto: &keyspb.PrivateKey{
					Der: keyDER,
				},
			},
			{
				Name: "PrivateKey with invalid DER",
				KeyProto: &keyspb.PrivateKey{
					Der: []byte("foobar"),
				},
				WantErr: true,
			},
			{
				Name:     "PrivateKey with missing DER",
				KeyProto: &keyspb.PrivateKey{},
				WantErr:  true,
			},
			// PKCS11Config support is tested by integration/log_integration.sh
			// (when $WITH_PKCS11 == "true").
		},
	}

	tester.RunAllTests(t)
}

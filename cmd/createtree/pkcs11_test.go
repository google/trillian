// +build pkcs11

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

package main

import (
	"os"
	"testing"

	"github.com/google/trillian/crypto/keyspb"
)

func TestRunPkcs11(t *testing.T) {
	err := os.Chdir("../..")
	if err != nil {
		t.Fatalf("Unable to change working directory to ../..: %s", err)
	}
	defer os.Chdir("cmd/createtree")

	pkcs11Tree := *defaultTree
	pkcs11Tree.PrivateKey = mustMarshalAny(&keyspb.PKCS11Config{
		TokenLabel: "log",
		Pin:        "1234",
		PublicKey: `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC7/tWwqUXZJaNfnpvnqiaeNMkn
hKusCsyAidrHxvuL+t54XFCHJwsB3wIlQZ4mMwb8mC/KRYhCqECBEoCAf/b0m3j/
ASuEPLyYOrz/aEs3wP02IZQLGmihmjMk7T/ouNCuX7y1fTjX3GeVQ06U/EePwZFC
xToc6NWBri0N3VVsswIDAQAB
-----END PUBLIC KEY-----
`,
	})

	runTest(t, []*testCase{
		{
			desc: "PKCS11ConfigFile",
			setFlags: func() {
				*privateKeyFormat = "PKCS11ConfigFile"
				*pkcs11ConfigPath = "testdata/pkcs11-conf.json"
			},
			wantErr:  false,
			wantTree: &pkcs11Tree,
		},
		{
			desc: "emptyPKCS11Path",
			setFlags: func() {
				*privateKeyFormat = "PKCS11ConfigFile"
				*pkcs11ConfigPath = ""
			},
			wantErr: true,
		},
	})
}

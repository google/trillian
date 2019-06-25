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
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keyspb"
)

func TestWithPEMKeyFile(t *testing.T) {
	pemPath, pemPassword := "../../testdata/log-rpc-server.privkey.pem", "towel"

	wantTree := proto.Clone(defaultTree).(*trillian.Tree)
	wantTree.PrivateKey = mustMarshalAny(&keyspb.PEMKeyFile{
		Path:     pemPath,
		Password: pemPassword,
	})

	runTest(t, []*testCase{
		{
			desc: "empty pemKeyPath",
			setFlags: func() {
				*privateKeyFormat = "PEMKeyFile"
				*pemKeyPath = ""
				*pemKeyPass = pemPassword
			},
			validateErr: errors.New("empty pem_key_path"),
			wantErr:     true,
		},
		{
			desc: "empty pemKeyPass",
			setFlags: func() {
				*privateKeyFormat = "PEMKeyFile"
				*pemKeyPath = pemPath
				*pemKeyPass = ""
			},
			validateErr: errors.New("pemfile: empty password for file"),
			wantErr:     true,
		},
		{
			desc: "valid pemKeyPath and pemKeyPass",
			setFlags: func() {
				*privateKeyFormat = "PEMKeyFile"
				*pemKeyPath = pemPath
				*pemKeyPass = pemPassword
			},
			wantTree: wantTree,
		},
	})
}

func TestWithPrivateKey(t *testing.T) {
	pemPath, pemPassword := "../../testdata/log-rpc-server.privkey.pem", "towel"

	key, err := pem.ReadPrivateKeyFile(pemPath, pemPassword)
	if err != nil {
		t.Fatalf("Error reading test private key file: %v", err)
	}

	keyDER, err := der.MarshalPrivateKey(key)
	if err != nil {
		t.Fatalf("Error marshaling test private key to DER: %v", err)
	}

	wantTree := proto.Clone(defaultTree).(*trillian.Tree)
	wantTree.PrivateKey = mustMarshalAny(&keyspb.PrivateKey{
		Der: keyDER,
	})

	runTest(t, []*testCase{
		{
			desc: "empty pemKeyPath",
			setFlags: func() {
				*privateKeyFormat = "PrivateKey"
				*pemKeyPath = ""
				*pemKeyPass = pemPassword
			},
			validateErr: errors.New("empty pem_key_path"),
			wantErr:     true,
		},
		{
			desc: "empty pemKeyPass",
			setFlags: func() {
				*privateKeyFormat = "PrivateKey"
				*pemKeyPath = pemPath
				*pemKeyPass = ""
			},
			validateErr: errors.New("pemfile: empty password for file"),
			wantErr:     true,
		},
		{
			desc: "valid pemKeyPath and pemKeyPass",
			setFlags: func() {
				*privateKeyFormat = "PrivateKey"
				*pemKeyPath = pemPath
				*pemKeyPass = pemPassword
			},
			wantTree: wantTree,
		},
	})
}

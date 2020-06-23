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

package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian/cmd/createtree/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keyspb"
)

var (
	pemKeyPath = flag.String("pem_key_path", "", "Path to the private key PEM file")
	pemKeyPass = flag.String("pem_key_password", "", "Password of the private key PEM file")
)

func init() {
	keys.RegisterType("PEMKeyFile", pemKeyFileProtoFromFlags)
	keys.RegisterType("PrivateKey", privateKeyProtoFromFlags)
}

func pemKeyFileProtoFromFlags() (proto.Message, error) {
	if *pemKeyPath == "" {
		return nil, errors.New("empty pem_key_path")
	}
	if *pemKeyPass == "" {
		return nil, fmt.Errorf("empty password for PEM key file %q", *pemKeyPath)
	}

	return &keyspb.PEMKeyFile{
		Path:     *pemKeyPath,
		Password: *pemKeyPass,
	}, nil
}

func privateKeyProtoFromFlags() (proto.Message, error) {
	if *pemKeyPath == "" {
		return nil, errors.New("empty pem_key_path")
	}

	key, err := pem.ReadPrivateKeyFile(*pemKeyPath, *pemKeyPass)
	if err != nil {
		return nil, fmt.Errorf("error reading reading private key file: %v", err)
	}

	keyDER, err := der.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("error marshaling private key as DER: %v", err)
	}

	return &keyspb.PrivateKey{Der: keyDER}, nil
}

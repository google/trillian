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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian/cmd/createtree/keys"
	"github.com/google/trillian/crypto/keyspb"

	pkcs11key "github.com/letsencrypt/pkcs11key/v4"
)

var pkcs11ConfigPath = flag.String("pkcs11_config_path", "", "Path to the PKCS #11 key configuration file")

func init() {
	keys.RegisterType("PKCS11ConfigFile", pkcs11ConfigProtoFromFlags)
}

func pkcs11ConfigProtoFromFlags() (proto.Message, error) {
	if *pkcs11ConfigPath == "" {
		return nil, errors.New("empty PKCS11 config file path")
	}

	configBytes, err := ioutil.ReadFile(*pkcs11ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading PKCS#11 config file: %v", err)
	}

	var config pkcs11key.Config
	if err = json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("error parsing PKCS#11 config file: %v", err)
	}

	pubKeyPEM, err := ioutil.ReadFile(config.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading PKCS#11 public key file: %v", err)
	}

	return &keyspb.PKCS11Config{
		TokenLabel: config.TokenLabel,
		Pin:        config.PIN,
		PublicKey:  string(pubKeyPEM),
	}, nil
}

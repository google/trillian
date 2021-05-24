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

package pem

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"google.golang.org/protobuf/proto"
)

// ReadPrivateKeyFile reads a PEM-encoded private key from a file.
// The key must be protected by a password.
func ReadPrivateKeyFile(file, password string) (crypto.Signer, error) {
	if password == "" {
		return nil, fmt.Errorf("pemfile: empty password for file %q", file)
	}

	keyPEM, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("pemfile: error reading file %q: %v", file, err)
	}

	k, err := UnmarshalPrivateKey(string(keyPEM), password)
	if err != nil {
		return nil, fmt.Errorf("pemfile: error decoding private key from file %q: %v", file, err)
	}

	return k, nil
}

// UnmarshalPrivateKey reads a PEM-encoded private key from a string.
// The key may be protected by a password.
func UnmarshalPrivateKey(keyPEM, password string) (crypto.Signer, error) {
	block, rest := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("pemfile: invalid private key PEM")
	}
	if len(rest) > 0 {
		return nil, errors.New("pemfile: extra data found after first PEM block")
	}

	keyDER := block.Bytes
	if password != "" {
		pwdDer, err := x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("pemfile: failed to decrypt: %v", err)
		}
		keyDER = pwdDer
	}

	return der.UnmarshalPrivateKey(keyDER)
}

// FromProto builds a crypto.Signer from a proto.Message, which must be of type PEMKeyFile.
func FromProto(_ context.Context, pb proto.Message) (crypto.Signer, error) {
	if pb, ok := pb.(*keyspb.PEMKeyFile); ok {
		return ReadPrivateKeyFile(pb.GetPath(), pb.GetPassword())
	}
	return nil, fmt.Errorf("pemfile: got %T, want *keyspb.PEMKeyFile", pb)
}

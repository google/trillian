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
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/crypto/keyspb"
)

// NewFromPEMKeyFileProto takes a PEMKeyFile protobuf message and loads the private key it specifies.
// It can be used as a ProtoHandler and passed to SignerFactory.AddHandler().
func NewFromPEMKeyFileProto(ctx context.Context, pb proto.Message) (crypto.Signer, error) {
	if f, ok := pb.(*keyspb.PEMKeyFile); ok {
		return NewFromPrivatePEMFile(f.GetPath(), f.GetPassword())
	}
	return nil, fmt.Errorf("pemfile: got %T, want *keyspb.PEMKeyFile", pb)
}

// NewFromPrivatePEMFile reads a PEM-encoded private key from a file.
// The key must be protected by a password.
func NewFromPrivatePEMFile(file, password string) (crypto.Signer, error) {
	if password == "" {
		return nil, fmt.Errorf("pemfile: empty password for file %q", file)
	}

	keyPEM, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("pemfile: error reading file %q: %v", file, err)
	}

	k, err := NewFromPrivatePEM(string(keyPEM), password)
	if err != nil {
		return nil, fmt.Errorf("pemfile: error decoding private key from file %q: %v", file, err)
	}

	return k, nil
}

// NewFromPublicPEMFile reads a PEM-encoded public key from a file.
func NewFromPublicPEMFile(file string) (crypto.PublicKey, error) {
	keyPEM, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("pemfile: error reading %q: %v", file, err)
	}

	return NewFromPublicPEM(string(keyPEM))
}

// NewFromPrivatePEM reads a PEM-encoded private key from a string.
// The key may be protected by a password.
func NewFromPrivatePEM(keyPEM, password string) (crypto.Signer, error) {
	block, rest := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("pemfile: invalid private key PEM")
	}
	if len(rest) > 0 {
		return nil, errors.New("pemfile: extra data found after first PEM block")
	}

	keyDER := block.Bytes
	if password != "" {
		pwdDer, err := x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return nil, fmt.Errorf("pemfile: failed to decrypt: %v", err)
		}
		keyDER = pwdDer
	}

	return NewFromPrivateDER(keyDER)
}

// NewFromPublicPEM reads a PEM-encoded public key from a string.
func NewFromPublicPEM(keyPEM string) (crypto.PublicKey, error) {
	block, rest := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("pemfile: invalid public key PEM")
	}
	if len(rest) > 0 {
		return nil, errors.New("pemfile: extra data found after first PEM block")
	}

	return NewFromPublicDER(block.Bytes)
}

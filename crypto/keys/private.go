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

package keys

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"

	"github.com/google/trillian"
	"github.com/google/trillian/errors"
)

// SignerFactory creates signers for Trillian trees.
// A signers may be created by loading a private key, interfacing with a HSM,
// or sending network requests to a remote key management service, to give a few
// examples.
type SignerFactory interface {
	// NewSigner returns a signer for the given tree.
	// The PrivateKey field of the tree controls how this is done.
	NewSigner(context.Context, *trillian.Tree) (crypto.Signer, error)
}

// Generator generates a new private key for Trillian trees.
// It is also a SignerFactory, i.e. it can create signers for trees that already have a key.
type Generator interface {
	SignerFactory

	// Generate creates a new private key for the given tree.
	// The SignatureAlgorithm and PrivateKey fields of the tree control
	// how this is done.
	Generate(context.Context, *trillian.Tree) error
}

// NewFromPrivatePEMFile reads a PEM-encoded private key from a file.
// The key may be protected by a password.
func NewFromPrivatePEMFile(keyFile, keyPassword string) (crypto.Signer, error) {
	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Errorf(errors.NotFound, "file does not exist: %q", keyFile)
		}
		return nil, errors.Errorf(errors.InvalidArgument, "failed to read private key from file %q: %v", keyFile, err)
	}

	k, err := NewFromPrivatePEM(string(pemData), keyPassword)
	if err != nil {
		return nil, errors.Errorf(errors.InvalidArgument, "failed to decode private key from file %q: %v", keyFile, err)
	}

	return k, nil
}

// NewFromPrivatePEM reads a PEM-encoded private key from a string.
// The key may be protected by a password.
func NewFromPrivatePEM(pemEncodedKey, password string) (crypto.Signer, error) {
	block, rest := pem.Decode([]byte(pemEncodedKey))
	if block == nil {
		return nil, errors.New(errors.InvalidArgument, "failed to decode PEM block")
	}
	if len(rest) > 0 {
		return nil, errors.New(errors.InvalidArgument, "extra data found after PEM decoding")
	}

	der := block.Bytes
	if password != "" {
		pwdDer, err := x509.DecryptPEMBlock(block, []byte(password))
		if err == x509.IncorrectPasswordError {
			return nil, errors.New(errors.PermissionDenied, err.Error())
		}
		if err != nil {
			return nil, err
		}
		der = pwdDer
	}

	return NewFromPrivateDER(der)
}

// NewFromPrivateDER reads a DER-encoded private key.
func NewFromPrivateDER(der []byte) (crypto.Signer, error) {
	key1, err1 := x509.ParsePKCS1PrivateKey(der)
	if err1 == nil {
		return key1, nil
	}

	key2, err2 := x509.ParsePKCS8PrivateKey(der)
	if err2 == nil {
		switch key2 := key2.(type) {
		case *ecdsa.PrivateKey:
			return key2, nil
		case *rsa.PrivateKey:
			return key2, nil
		}
		return nil, errors.Errorf(errors.InvalidArgument, "unsupported private key type: %T", key2)
	}

	key3, err3 := x509.ParseECPrivateKey(der)
	if err3 == nil {
		return key3, nil
	}

	return nil, errors.Errorf(errors.InvalidArgument, "could not parse DER private key as PKCS1 (%v), PKCS8 (%v), or SEC1 (%v)", err1, err2, err3)
}

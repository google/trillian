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
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/google/trillian/crypto/sigpb"
)

// PrivateKeyManager supports signing data with a private key that may be stored
// in a secure location, which is not immediately available to this client.
type PrivateKeyManager interface {
	// Signer implements a crypto.Signer that can sign data using the private key.
	crypto.Signer
	// SignatureAlgorithm returns the value that identifies the signature algorithm.
	SignatureAlgorithm() sigpb.DigitallySigned_SignatureAlgorithm
}

// localSigner signs objects using in-memory key material.
type localSigner struct {
	crypto.Signer
	signatureAlgorithm sigpb.DigitallySigned_SignatureAlgorithm
}

// SignatureAlgorithm identifies the signature algorithm used by this key manager.
func (k localSigner) SignatureAlgorithm() sigpb.DigitallySigned_SignatureAlgorithm {
	return k.signatureAlgorithm
}

// NewFromPrivateKey creates PrivateKeyManager using a private key.
func NewFromPrivateKey(key crypto.PrivateKey) (PrivateKeyManager, error) {
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		return &localSigner{
			Signer:             key,
			signatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		}, nil
	case *rsa.PrivateKey:
		return &localSigner{
			Signer:             key,
			signatureAlgorithm: sigpb.DigitallySigned_RSA,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
}

func parsePrivateKey(key []byte) (crypto.PrivateKey, error) {
	key1, err1 := x509.ParsePKCS1PrivateKey(key)
	if err1 == nil {
		return key1, nil
	}
	key2, err2 := x509.ParsePKCS8PrivateKey(key)
	if err2 == nil {
		return key2, nil
	}
	key3, err3 := x509.ParseECPrivateKey(key)
	if err3 == nil {
		return key3, nil
	}
	return nil, fmt.Errorf("could not parse private key as PKCS1: %v, PKCS8: %v, or SEC1: %v", err1, err2, err3)
}

// NewFromPrivatePEM returns key manager for a PEM object which may be password protected.
func NewFromPrivatePEM(pemBlock, password string) (PrivateKeyManager, error) {
	block, rest := pem.Decode([]byte(pemBlock))
	if len(rest) > 0 {
		return nil, errors.New("extra data found after PEM decoding")
	}

	der := block.Bytes
	if password != "" {
		pwdDer, err := x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return nil, err
		}
		der = pwdDer
	}

	key, err := parsePrivateKey(der)
	if err != nil {
		return nil, err
	}
	return NewFromPrivateKey(key)
}

// NewFromPrivatePEMFile initializes and returns a new PrivateKeyManager using a PEM encoded
// private key read from a file. The key may be protected by a password.
func NewFromPrivatePEMFile(keyFile, keyPassword string) (PrivateKeyManager, error) {
	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from key file: %s because: %v", keyFile, err)
	}
	return NewFromPrivatePEM(string(pemData), keyPassword)
}

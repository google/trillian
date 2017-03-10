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

// Package keys provides access to public and private keys for signing and verification of signatures.
package keys

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

// NewFromPublicPEMFile reads a PEM-encoded public key from a file.
func NewFromPublicPEMFile(keyFile string) (crypto.PublicKey, error) {
	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s. %v", keyFile, err)
	}
	return NewFromPublicPEM(string(pemData))
}

// NewFromPublicPEM reads a PEM-encoded public key from a string.
func NewFromPublicPEM(pemEncodedKey string) (crypto.PublicKey, error) {
	publicBlock, rest := pem.Decode([]byte(pemEncodedKey))
	if publicBlock == nil {
		return nil, errors.New("could not decode PEM for public key")
	}
	if len(rest) > 0 {
		return nil, errors.New("extra data found after PEM key decoded")
	}

	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse public key: %v", err)
	}

	return parsedKey, nil
}

// SignatureAlgorithm returns the algorithm used for this public key.
// Only ECDSA and RSA keys are supported. Other key types will return sigpb.DigitallySigned_UNKNOWN.
func SignatureAlgorithm(k crypto.PublicKey) sigpb.DigitallySigned_SignatureAlgorithm {
	switch k.(type) {
	case *ecdsa.PublicKey:
		return sigpb.DigitallySigned_ECDSA
	case *rsa.PublicKey:
		return sigpb.DigitallySigned_RSA
	}

	return sigpb.DigitallySigned_ANONYMOUS
}

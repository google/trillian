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
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
)

// NewFromPrivatePEMFile reads a PEM-encoded private key from a file.
// The key may be protected by a password.
func NewFromPrivatePEMFile(keyFile, keyPassword string) (crypto.Signer, error) {
	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key from file %q: %v", keyFile, err)
	}

	k, err := NewFromPrivatePEM(string(pemData), keyPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key from file %q: %v", keyFile, err)
	}

	return k, nil
}

// NewFromPrivatePEM reads a PEM-encoded private key from a string.
// The key may be protected by a password.
func NewFromPrivatePEM(pemEncodedKey, password string) (crypto.Signer, error) {
	block, rest := pem.Decode([]byte(pemEncodedKey))
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

	return parsePrivateKey(der)
}

func parsePrivateKey(rawKey []byte) (crypto.Signer, error) {
	key1, err1 := x509.ParsePKCS1PrivateKey(rawKey)
	if err1 == nil {
		return key1, nil
	}

	key2, err2 := x509.ParsePKCS8PrivateKey(rawKey)
	if err2 == nil {
		switch key2 := key2.(type) {
		case *ecdsa.PrivateKey:
			return key2, nil
		case *rsa.PrivateKey:
			return key2, nil
		}
		return nil, fmt.Errorf("got %T, want *{ecdsa,rsa}.PrivateKey", key2)
	}

	key3, err3 := x509.ParseECPrivateKey(rawKey)
	if err3 == nil {
		return key3, nil
	}

	return nil, fmt.Errorf("could not parse private key as PKCS1 (%v), PKCS8 (%v), or SEC1 (%v)", err1, err2, err3)
}

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
	// Signer returns a crypto.Signer that can sign data using the private key.
	Signer() crypto.Signer
	// SignatureAlgorithm returns the value that identifies the signature algorithm.
	SignatureAlgorithm() sigpb.DigitallySigned_SignatureAlgorithm
	// HashAlgorithm returns the type of hash that will be used for signing with this key.
	HashAlgorithm() crypto.Hash
	// PublicKey returns the public key corresponding to the private key.
	PublicKey() crypto.PublicKey
}

// PEMKeyManager is an instance of KeyManager that loads its key data from an encrypted
// PEM file.
type PEMKeyManager struct {
	signer             crypto.Signer
	signatureAlgorithm sigpb.DigitallySigned_SignatureAlgorithm
}

// SignatureAlgorithm identifies the signature algorithm used by this key manager.
func (k PEMKeyManager) SignatureAlgorithm() sigpb.DigitallySigned_SignatureAlgorithm {
	return k.signatureAlgorithm
}

// HashAlgorithm identifies the hash algorithm used to sign objects.
func (k PEMKeyManager) HashAlgorithm() crypto.Hash {
	// TODO: Save the hash algorithm in the key serialization.
	// Return a default hash algorithm for now.
	return crypto.SHA256
}

// Signer returns a signer based on our private key.
func (k PEMKeyManager) Signer() crypto.Signer {
	return k.signer
}

// PublicKey returns the public key corresponding to the private key.
func (k PEMKeyManager) PublicKey() crypto.PublicKey {
	return k.signer.Public()
}

// NewFromPrivateKey creates PrivateKeyManager using a private key.
func NewFromPrivateKey(key crypto.PrivateKey) (PrivateKeyManager, error) {
	var signer crypto.Signer
	var sigAlgo sigpb.DigitallySigned_SignatureAlgorithm

	switch key.(type) {
	case *ecdsa.PrivateKey:
		signer = key.(crypto.Signer)
		sigAlgo = sigpb.DigitallySigned_ECDSA
	case *rsa.PrivateKey:
		signer = key.(crypto.Signer)
		sigAlgo = sigpb.DigitallySigned_RSA
	default:
		return nil, errors.New("unsupported key type")
	}

	return &PEMKeyManager{
		signer:             signer,
		signatureAlgorithm: sigAlgo,
	}, nil
}

func parsePrivateKey(key []byte) (crypto.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(key); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(key); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(key); err == nil {
		return key, nil
	}
	return nil, errors.New("could not parse private key")
}

// NewFromPrivatePEM returns key manager for a password protected PEM object.
func NewFromPrivatePEM(pemBlock []byte, password string) (PrivateKeyManager, error) {
	block, rest := pem.Decode(pemBlock)
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

// NewFromPrivatePEMFile initializes and returns a new KeyManager using a PEM encoded
// private key read from a file. The key may be protected by a password.
func NewFromPrivatePEMFile(keyFile, keyPassword string) (PrivateKeyManager, error) {
	if len(keyFile) == 0 || len(keyPassword) == 0 {
		return nil, errors.New("private key file and password must be specified")
	}

	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from key file: %s because: %v", keyFile, err)
	}
	return NewFromPrivatePEM(pemData, keyPassword)
}

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

	"github.com/golang/glog"
	"github.com/google/trillian/crypto/sigpb"
)

// KeyManager loads and holds our private and public keys. Should support ECDSA and RSA keys.
// The crypto.Signer API allows for obtaining a public key from a private key but there are
// cases where we have the public key only, such as mirroring another log, so we treat them
// separately. KeyManager is an interface as we expect multiple implementations supporting
// different ways of accessing keys.
type KeyManager interface {
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
	serverPrivateKey   crypto.PrivateKey
	signatureAlgorithm sigpb.DigitallySigned_SignatureAlgorithm
	rawPublicKey       []byte
}

// NewPEMKeyManager creates an uninitialized PEMKeyManager. Keys must be loaded before it
// can be used
func NewPEMKeyManager() *PEMKeyManager {
	return &PEMKeyManager{}
}

// NewPEMKeyManager creates a PEMKeyManager using a private key that has already been loaded
func (k PEMKeyManager) NewPEMKeyManager(key crypto.PrivateKey) *PEMKeyManager {
	return &PEMKeyManager{serverPrivateKey: key}
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

// LoadPrivateKey loads a private key from a PEM encoded string, decrypting it if necessary
func (k *PEMKeyManager) LoadPrivateKey(pemEncodedKey, password string) error {
	block, rest := pem.Decode([]byte(pemEncodedKey))
	if len(rest) > 0 {
		return errors.New("extra data found after PEM decoding")
	}

	der := block.Bytes
	if password != "" {
		pwdDer, err := x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return err
		}
		der = pwdDer
	}

	key, algo, err := parsePrivateKey(der)
	if err != nil {
		return err
	}

	switch key.(type) {
	case *ecdsa.PrivateKey, *rsa.PrivateKey:
		k.signer = key.(crypto.Signer)
	default:
		return errors.New("unsupported key type")
	}

	k.serverPrivateKey = key
	k.signatureAlgorithm = algo
	return nil
}

// Signer returns a signer based on our private key.
func (k PEMKeyManager) Signer() crypto.Signer {
	return k.signer
}

// PublicKey returns the public key corresponding to the private key.
func (k PEMKeyManager) PublicKey() crypto.PublicKey {
	return k.signer.Public()
}

func parsePrivateKey(key []byte) (crypto.PrivateKey, sigpb.DigitallySigned_SignatureAlgorithm, error) {
	// Our two ways of reading keys are ParsePKCS1PrivateKey and ParsePKCS8PrivateKey.
	// And ParseECPrivateKey. Our three ways of parsing keys are ... I'll come in again.
	if key, err := x509.ParsePKCS1PrivateKey(key); err == nil {
		return key, sigpb.DigitallySigned_RSA, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(key); err == nil {
		switch key := key.(type) {
		case *ecdsa.PrivateKey:
			return key, sigpb.DigitallySigned_ECDSA, nil
		case *rsa.PrivateKey:
			return key, sigpb.DigitallySigned_RSA, nil
		default:
			return nil, sigpb.DigitallySigned_ANONYMOUS, fmt.Errorf("unknown private key type: %T", key)
		}
	}
	var err error
	if key, err := x509.ParseECPrivateKey(key); err == nil {
		return key, sigpb.DigitallySigned_ECDSA, nil
	}

	glog.Warningf("error parsing EC key: %s", err)
	return nil, sigpb.DigitallySigned_ANONYMOUS, errors.New("could not parse private key")
}

// LoadPasswordProtectedPrivateKey initializes and returns a new KeyManager using a PEM encoded
// private key read from a file. The key may be protected by a password.
func LoadPasswordProtectedPrivateKey(keyFile, keyPassword string) (KeyManager, error) {
	if len(keyFile) == 0 || len(keyPassword) == 0 {
		return nil, errors.New("private key file and password must be specified")
	}

	pemData, err := ioutil.ReadFile(keyFile)

	if err != nil {
		return nil, fmt.Errorf("failed to read data from key file: %s because: %v", keyFile, err)
	}

	km := NewPEMKeyManager()
	err = km.LoadPrivateKey(string(pemData[:]), keyPassword)

	if err != nil {
		return nil, err
	}

	return *km, nil
}

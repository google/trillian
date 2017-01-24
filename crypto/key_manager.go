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
	"github.com/google/trillian"
)

// KeyManager loads and holds our private and public keys. Should support ECDSA and RSA keys.
// The crypto.Signer API allows for obtaining a public key from a private key but there are
// cases where we have the public key only, such as mirroring another log, so we treat them
// separately. KeyManager is an interface as we expect multiple implementations supporting
// different ways of accessing keys.
type KeyManager interface {
	// Signer returns a crypto.Signer that can sign data using the private key held by the
	// manager.
	Signer() (crypto.Signer, error)
	// SignatureAlgorithm returns the value that identifies the signature algorithm.
	SignatureAlgorithm() trillian.SignatureAlgorithm
	// GetPublicKey returns the public key previously loaded. It is an error to call this
	// before a public key has been loaded
	GetPublicKey() (crypto.PublicKey, error)
	// GetRawPublicKey returns the DER encoded public key bytes.
	// This is needed for some applications that exchange or embed key hashes in structures.
	// It is an error to call this before a public key has been loaded
	GetRawPublicKey() ([]byte, error)
}

// PEMKeyManager is an instance of KeyManager that loads its key data from an encrypted
// PEM file.
type PEMKeyManager struct {
	serverPrivateKey   crypto.PrivateKey
	signatureAlgorithm trillian.SignatureAlgorithm
	serverPublicKey    crypto.PublicKey
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
func (k PEMKeyManager) SignatureAlgorithm() trillian.SignatureAlgorithm {
	return k.signatureAlgorithm
}

// LoadPrivateKey loads a private key from a PEM encoded string, decrypting it if necessary
func (k *PEMKeyManager) LoadPrivateKey(pemEncodedKey, password string) error {
	block, rest := pem.Decode([]byte(pemEncodedKey))
	if len(rest) > 0 {
		return errors.New("extra data found after PEM decoding")
	}

	der, err := x509.DecryptPEMBlock(block, []byte(password))
	if err != nil {
		return err
	}

	key, algo, err := parsePrivateKey(der)
	if err != nil {
		return err
	}

	k.serverPrivateKey = key
	k.signatureAlgorithm = algo
	return nil
}

// LoadPublicKey loads a public key from a PEM encoded string.
func (k *PEMKeyManager) LoadPublicKey(pemEncodedKey string) error {
	publicBlock, rest := pem.Decode([]byte(pemEncodedKey))
	if publicBlock == nil {
		return errors.New("could not decode PEM for public key")
	}
	if len(rest) > 0 {
		return errors.New("extra data found after PEM key decoded")
	}

	k.rawPublicKey = publicBlock.Bytes

	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return errors.New("unable to parse public key")
	}

	k.serverPublicKey = parsedKey
	return nil
}

// Signer returns a signer based on our private key. Returns an error if no private key
// has been loaded.
func (k PEMKeyManager) Signer() (crypto.Signer, error) {
	if k.serverPrivateKey == nil {
		return nil, errors.New("private key is not loaded")
	}

	// Good old interface{}, this wouldn't be necessary in a proper type system. If it's
	// even the right thing to do but I couldn't find any useful docs so meh
	switch k.serverPrivateKey.(type) {
	case *ecdsa.PrivateKey, *rsa.PrivateKey:
		return k.serverPrivateKey.(crypto.Signer), nil
	}

	return nil, errors.New("unsupported key type")
}

// GetPublicKey returns the public key previously loaded or an error if LoadPublicKey has
// not been previously called successfully.
func (k PEMKeyManager) GetPublicKey() (crypto.PublicKey, error) {
	if k.serverPublicKey == nil {
		return nil, errors.New("called GetPublicKey() but one is not loaded")
	}

	return k.serverPublicKey, nil
}

// GetRawPublicKey returns the DER encoded public key bytes as loaded from the file.
// This is needed for some applications that exchange or embed key hashes in structures.
// The result will be an error if a public key has not been loaded
func (k PEMKeyManager) GetRawPublicKey() ([]byte, error) {
	if k.rawPublicKey == nil {
		return nil, errors.New("called GetRawPublicKey() but one is not loaded")
	}

	return k.rawPublicKey, nil
}

func parsePrivateKey(key []byte) (crypto.PrivateKey, trillian.SignatureAlgorithm, error) {
	// Our two ways of reading keys are ParsePKCS1PrivateKey and ParsePKCS8PrivateKey.
	// And ParseECPrivateKey. Our three ways of parsing keys are ... I'll come in again.
	if key, err := x509.ParsePKCS1PrivateKey(key); err == nil {
		return key, trillian.SignatureAlgorithm_RSA, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(key); err == nil {
		switch key := key.(type) {
		case *ecdsa.PrivateKey:
			return key, trillian.SignatureAlgorithm_ECDSA, nil
		case *rsa.PrivateKey:
			return key, trillian.SignatureAlgorithm_RSA, nil
		default:
			return nil, trillian.SignatureAlgorithm_ANONYMOUS, fmt.Errorf("unknown private key type: %T", key)
		}
	}
	var err error
	if key, err := x509.ParseECPrivateKey(key); err == nil {
		return key, trillian.SignatureAlgorithm_ECDSA, nil
	}

	glog.Warningf("error parsing EC key: %s", err)
	return nil, trillian.SignatureAlgorithm_ANONYMOUS, errors.New("could not parse private key")
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

package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// KeyManager loads and holds our private keys. Should support ECDSA and RSA keys.
type KeyManager struct {
	serverPrivateKey crypto.PrivateKey
}

// NewKeyManager creates a key manager using a private key that has already been loaded
func (k KeyManager) NewKeyManager(key crypto.PrivateKey) *KeyManager {
	return &KeyManager{key}
}

// LoadPrivateKey loads a private key from a PEM encoded string, decrypting it if necessary
func (k *KeyManager) LoadPrivateKey(pemEncodedKey, password string) error {
	block, rest := pem.Decode([]byte(pemEncodedKey))
	if len(rest) > 0 {
		return fmt.Errorf("Extra data found after PEM decoding")
	}

	der, err := x509.DecryptPEMBlock(block, []byte(password))

	if err != nil {
		return err
	}

	key, err := parsePrivateKey(der)

	if err != nil {
		return err
	}

	k.serverPrivateKey = key
	return nil
}

// Sign signs a block of bytes and returns the results. We don't allow callers access
// to the key
func (k KeyManager) Signer() (crypto.Signer, error) {
	if k.serverPrivateKey == nil {
		//return nil, fmt.Errorf("Private key is not loaded")
	}

	// Good old interface{}, this wouldn't be necessary in a proper type system. If it's
	// even the right thing to do but I couldn't find any useful docs so meh
	switch k.serverPrivateKey.(type) {
	case *ecdsa.PrivateKey, *rsa.PrivateKey:
		return k.serverPrivateKey.(crypto.Signer), nil
	}

	return nil, fmt.Errorf("Unsupported key type")
}

func parsePrivateKey(key []byte) (crypto.PrivateKey, error) {
	// Our two ways of reading keys are ParsePKCS1PrivateKey and ParsePKCS8PrivateKey.
	// And ParseECPrivateKey. Our three ways of parsing keys are ... I'll come in again.
	if key, err := x509.ParsePKCS1PrivateKey(key); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(key); err == nil {
		switch key := key.(type) {
		case *ecdsa.PrivateKey, *rsa.PrivateKey:
			return key, nil
		default:
			return nil, errors.New("Unknown private key type")
		}
	}
	if key, err := x509.ParseECPrivateKey(key); err == nil {
		return key, nil
	}

	return nil, errors.New("Could not parse private key")
}

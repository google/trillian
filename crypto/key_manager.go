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

// KeyManager loads and holds our private and public keys. Should support ECDSA and RSA keys.
// The crypto.Signer API allows for obtaining a public key from a private key but there are
// cases where we have the public key only, such as mirroring another log, so we treat them
// separately.
type KeyManager struct {
	serverPrivateKey crypto.PrivateKey
	serverPublicKey crypto.PublicKey
}

// NewKeyManager creates an uninitialized KeyManager. Keys must be loaded before it
// can be used
func NewKeyManager() *KeyManager {
	return &KeyManager{}
}

// NewKeyManager creates a key manager using a private key that has already been loaded
func NewKeyManagerWithKey(key crypto.PrivateKey) *KeyManager {
	return &KeyManager{key, nil}
}

// LoadPrivateKey loads a private key from a PEM encoded string, decrypting it if necessary
func (k *KeyManager) LoadPrivateKey(pemEncodedKey, password string) error {
	block, rest := pem.Decode([]byte(pemEncodedKey))
	if len(rest) > 0 {
		return fmt.Errorf("extra data found after PEM decoding")
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

// LoadPublicKey loads a public key from a PEM encoded string.
func (k *KeyManager) LoadPublicKey(pemEncodedKey string) error {
	publicBlock, rest := pem.Decode([]byte(pemEncodedKey))

	if publicBlock == nil {
		return errors.New("could not decode PEM for public key")
	}

	if len(rest) > 0 {
		return errors.New("extra data found after PEM key decoded")
	}

	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)

	if err != nil {
		return errors.New("unable to parse public key")
	}

	k.serverPublicKey = parsedKey
	return nil
}

// Signer returns a signer based on our private key. Returns nil if no private key
// has been loaded.
func (k KeyManager) Signer() (crypto.Signer, error) {
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

// GetPublicKey returns the public key previously loaded or nil if LoadPublicKey has
// not been previously called successfully.
func (k KeyManager) GetPublicKey() crypto.PublicKey {
	return k.serverPublicKey
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
			return nil, fmt.Errorf("unknown private key type: %T", key)
		}
	}
	if key, err := x509.ParseECPrivateKey(key); err == nil {
		return key, nil
	}

	return nil, errors.New("could not parse private key")
}

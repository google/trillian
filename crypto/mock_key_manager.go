package crypto

import (
	"crypto"
	"io"

	"github.com/stretchr/testify/mock"
)

// Mock for KeyManager
type MockKeyManager struct {
	mock.Mock
}

// Mock for crypto.Signer
type MockSigner struct {
	mock.Mock
}

// Signer is a Mock
func (m MockKeyManager) Signer() (crypto.Signer, error) {
	args := m.Called()
	return args.Get(0).(crypto.Signer), args.Error(1)
}

func (m MockKeyManager) GetPublicKey() crypto.PublicKey {
	args := m.Called()
	return args.Get(0).(crypto.PublicKey)
}

func (m MockSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	args := m.Called(rand, digest, opts)
	return args.Get(0).([]byte), args.Error(1)
}

func (m MockSigner) Public() crypto.PublicKey {
	args := m.Called()
	return args.Get(0).(crypto.PublicKey)
}
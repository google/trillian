package log

import (
	"bytes"
	"crypto"
	"errors"
	"io"
	"testing"

	"github.com/google/trillian"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const message string = "testing"
const result string = "echo"

type mockSigner struct {
	mock.Mock
}

func (m mockSigner) Sign(rand io.Reader, msg []byte, opts crypto.SignerOpts) ([]byte, error) {
	args := m.Called(rand, msg, opts)

	return args.Get(0).([]byte), args.Error(1)
}

func (m mockSigner) Public() crypto.PublicKey {
	args := m.Called("Public")

	return args.Get(0)
}

func messageHash() []byte {
	h := trillian.NewSHA256()
	return h.Digest([]byte(message))
}

func TestSigner(t *testing.T) {
	mockSigner := new(mockSigner)
	digest := messageHash()

	mockSigner.On("Sign",
		mock.MatchedBy(func(io.Reader) bool { return true }),
		digest,
		mock.MatchedBy(func(s crypto.SignerOpts) bool {
			return s.HashFunc() == crypto.SHA256
		})).Return([]byte(result), nil)

	hasher, err := trillian.NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		t.Fatalf("Failed to create new hasher: %s", err)
	}
	logSigner := NewSigner(hasher, trillian.SignatureAlgorithm_RSA, mockSigner)

	sig, err := logSigner.Sign([]byte(message))

	if err != nil {
		t.Fatalf("Failed to sign: %s", err)
	}

	mockSigner.AssertExpectations(t)

	assert.True(t, sig.HashAlgorithm == trillian.HashAlgorithm_SHA256, "Hash alg incorrect: %v", sig.HashAlgorithm)
	assert.True(t, sig.SignatureAlgorithm == trillian.SignatureAlgorithm_RSA, "Sig alg incorrect: %v", sig.SignatureAlgorithm)
	assert.Zero(t, bytes.Compare([]byte(result), sig.Signature), "Mismatched sig: [%v] [%v]",
		[]byte(result), sig.Signature)
}

func TestSignerFails(t *testing.T) {
	mockSigner := new(mockSigner)
	digest := messageHash()

	mockSigner.On("Sign",
		mock.MatchedBy(func(io.Reader) bool { return true }),
		digest[:],
		mock.MatchedBy(func(s crypto.SignerOpts) bool { return s.HashFunc() == crypto.SHA256 })).Return(digest, errors.New("sign"))

	hasher, err := trillian.NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		t.Fatalf("Failed to create new hasher: %s", err)
	}
	logSigner := NewSigner(hasher, trillian.SignatureAlgorithm_RSA, mockSigner)

	_, err = logSigner.Sign([]byte(message))

	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	ensureErrorContains(t, err, "sign")

	mockSigner.AssertExpectations(t)
}

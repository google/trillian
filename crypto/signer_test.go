package crypto

import (
	"bytes"
	"crypto"
	"errors"
	"io"
	"strings"
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

	logSigner := createTestSigner(t, *mockSigner)

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

	logSigner := createTestSigner(t, *mockSigner)

	_, err := logSigner.Sign([]byte(message))

	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	ensureErrorContains(t, err, "sign")

	mockSigner.AssertExpectations(t)
}

func TestSignLogRootSignerFails(t *testing.T) {
	mockSigner := new(mockSigner)

	mockSigner.On("Sign",
		mock.MatchedBy(func(io.Reader) bool {
			return true
		}),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		mock.MatchedBy(func(s crypto.SignerOpts) bool {
			return s.HashFunc() == crypto.SHA256
		})).Return([]byte{}, errors.New("signfail"))

	logSigner := createTestSigner(t, *mockSigner)

	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	err := logSigner.SignLogRoot(&root)

	ensureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	mockSigner := new(mockSigner)

	mockSigner.On("Sign",
		mock.MatchedBy(func(io.Reader) bool {
			return true
		}),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		mock.MatchedBy(func(s crypto.SignerOpts) bool {
			return s.HashFunc() == crypto.SHA256
		})).Return([]byte(result), nil)

	logSigner := createTestSigner(t, *mockSigner)

	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	err := logSigner.SignLogRoot(&root)

	if err != nil {
		t.Fatalf("Failed to sign log root: %v", err)
	}

	expected := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2,
		Signature: &trillian.DigitallySigned{SignatureAlgorithm:trillian.SignatureAlgorithm_RSA,
			HashAlgorithm: trillian.HashAlgorithm_SHA256,
			Signature: []byte("echo")}}
	assert.Equal(t, expected, root, "Expected a correctly signed root")
}

// TODO(Martin2112): Tidy up so we only have one copy of this
func ensureErrorContains(t *testing.T, err error, s string) {
	if err == nil {
		t.Fatalf("%s operation unexpectedly succeeded", s)
	}

	if !strings.Contains(err.Error(), s) {
		t.Errorf("Got the wrong type of error: %v", err)
	}
}

func createTestSigner(t *testing.T, mock mockSigner) *Signer {
	hasher, err := trillian.NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		t.Fatalf("Failed to create new hasher: %s", err)
	}

	return NewSigner(hasher, trillian.SignatureAlgorithm_RSA, mock)
}
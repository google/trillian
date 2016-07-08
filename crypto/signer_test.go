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

func TestLogRootToJson(t *testing.T) {
	mockSigner := new(mockSigner)
	logSigner := createTestSigner(t, *mockSigner)
	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	jsonRoot, err := logSigner.LogRootToJson(root)

	if err != nil {
		t.Fatalf("Error in serializing root: %v", err)
	}

	// Binary data like the root hash gets base64 encoded
	expected := `{"TimestampNanos":2267709,"TreeSize":2,"RootHash":"SXNsaW5ndG9u"}`
	assert.Equal(t, expected, jsonRoot, "Incorrect log root serialization format")
	mockSigner.AssertExpectations(t)
}

func TestSignLogRootSignerFails(t *testing.T) {
	mockSigner := new(mockSigner)

	mockSigner.On("Sign",
		mock.MatchedBy(func(io.Reader) bool {
			return true
		}),
		[]byte{0x53, 0x71, 0x31, 0x25, 0x6, 0x6d, 0xc7, 0x66, 0xa4, 0xdf, 0x1f, 0xd3, 0x4b, 0x40, 0x73, 0x1e, 0x4a, 0xc0, 0x50, 0xa2, 0xf9, 0x5c, 0x54, 0x7b, 0xa2, 0xea, 0x83, 0xa1, 0xf0, 0xb6, 0x8c, 0xea},
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
		[]byte{0x53, 0x71, 0x31, 0x25, 0x6, 0x6d, 0xc7, 0x66, 0xa4, 0xdf, 0x1f, 0xd3, 0x4b, 0x40, 0x73, 0x1e, 0x4a, 0xc0, 0x50, 0xa2, 0xf9, 0x5c, 0x54, 0x7b, 0xa2, 0xea, 0x83, 0xa1, 0xf0, 0xb6, 0x8c, 0xea},
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
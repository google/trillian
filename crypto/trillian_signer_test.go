package crypto

import (
	"bytes"
	"crypto"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/stretchr/testify/mock"
)

const message string = "testing"
const result string = "echo"

type mockTrillianSigner struct {
	mock.Mock
}

// Sign is a mock
func (m mockTrillianSigner) Sign(rand io.Reader, msg []byte, opts crypto.SignerOpts) ([]byte, error) {
	args := m.Called(rand, msg, opts)

	return args.Get(0).([]byte), args.Error(1)
}

// Public is a mock
func (m mockTrillianSigner) Public() crypto.PublicKey {
	args := m.Called()

	return args.Get(0)
}

func messageHash() []byte {
	h := trillian.NewSHA256()
	return h.Digest([]byte(message))
}

func TestSigner(t *testing.T) {
	mockSigner := new(mockTrillianSigner)
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

	if got, want := sig.HashAlgorithm, trillian.HashAlgorithm_SHA256; got != want {
		t.Fatalf("Hash alg incorrect, got %v expected %d", got, want)
	}
	if got, want := sig.SignatureAlgorithm, trillian.SignatureAlgorithm_RSA; got != want {
		t.Fatalf("Sig alg incorrect, got %v expected %v", got, want)
	}
	if got, want := []byte(result), sig.Signature; !bytes.Equal(got, want) {
		t.Fatalf("Mismatched sig got [%v] expected [%v]", got, want)
	}
}

func TestSignerFails(t *testing.T) {
	mockSigner := new(mockTrillianSigner)
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

	testonly.EnsureErrorContains(t, err, "sign")

	mockSigner.AssertExpectations(t)
}

func TestSignLogRootSignerFails(t *testing.T) {
	mockSigner := new(mockTrillianSigner)

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
	_, err := logSigner.SignLogRoot(root)

	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	mockSigner := new(mockTrillianSigner)

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
	signature, err := logSigner.SignLogRoot(root)

	if err != nil {
		t.Fatalf("Failed to sign log root: %v", err)
	}

	// Check root is not modified
	expected := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	if !reflect.DeepEqual(root, expected) {
		t.Fatalf("Got %v, but expected unmodified signed root %v", root, expected)
	}
	// And signature is correct
	expectedSignature := trillian.DigitallySigned{SignatureAlgorithm: trillian.SignatureAlgorithm_RSA,
		HashAlgorithm: trillian.HashAlgorithm_SHA256,
		Signature:     []byte("echo")}
	if !reflect.DeepEqual(signature, expectedSignature) {
		t.Fatalf("Got %v, but expected %v", signature, expectedSignature)
	}
}

func createTestSigner(t *testing.T, mock mockTrillianSigner) *TrillianSigner {
	hasher, err := trillian.NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		t.Fatalf("Failed to create new hasher: %s", err)
	}

	return NewTrillianSigner(hasher, trillian.SignatureAlgorithm_RSA, mock)
}

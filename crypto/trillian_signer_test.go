package crypto

import (
	"bytes"
	"crypto"
	"errors"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
)

const message string = "testing"
const result string = "echo"

func messageHash() []byte {
	h := NewSHA256()
	return h.Digest([]byte(message))
}

type usesSHA256Hasher struct{}

func (i usesSHA256Hasher) Matches(x interface{}) bool {
	h, ok := x.(crypto.SignerOpts)
	if !ok {
		return false
	}
	return h.HashFunc() == crypto.SHA256
}

func (i usesSHA256Hasher) String() string {
	return "uses SHA256 hasher"
}

func TestSigner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)
	digest := messageHash()

	mockSigner.EXPECT().Sign(gomock.Any(), digest, usesSHA256Hasher{}).Return([]byte(result), nil)

	logSigner := createTestSigner(t, mockSigner)

	sig, err := logSigner.Sign([]byte(message))

	if err != nil {
		t.Fatalf("Failed to sign: %s", err)
	}

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)
	digest := messageHash()

	mockSigner.EXPECT().Sign(gomock.Any(), digest[:], usesSHA256Hasher{}).Return(digest, errors.New("sign"))

	logSigner := createTestSigner(t, mockSigner)

	_, err := logSigner.Sign([]byte(message))

	if err == nil {
		t.Fatalf("Ignored a signing error: %v", err)
	}

	testonly.EnsureErrorContains(t, err, "sign")
}

func TestSignLogRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)

	mockSigner.EXPECT().Sign(gomock.Any(),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		usesSHA256Hasher{}).Return([]byte{}, errors.New("signfail"))

	logSigner := createTestSigner(t, mockSigner)

	root := trillian.SignedLogRoot{TimestampNanos: 2267709, RootHash: []byte("Islington"), TreeSize: 2}
	_, err := logSigner.SignLogRoot(root)

	testonly.EnsureErrorContains(t, err, "signfail")
}

func TestSignLogRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSigner := NewMockSigner(ctrl)

	mockSigner.EXPECT().Sign(gomock.Any(),
		[]byte{0xe5, 0xb3, 0x18, 0x1a, 0xec, 0xc8, 0x64, 0xc6, 0x39, 0x6d, 0x83, 0x21, 0x7a, 0x18, 0x3, 0x9, 0xf5, 0xa0, 0x25, 0xde, 0xf7, 0x1b, 0xdb, 0x2d, 0xbe, 0x42, 0x8a, 0x4a, 0xab, 0xc1, 0xcd, 0x49},
		usesSHA256Hasher{}).Return([]byte(result), nil)

	logSigner := createTestSigner(t, mockSigner)

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

func createTestSigner(t *testing.T, mock *MockSigner) *TrillianSigner {
	hasher, err := NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		t.Fatalf("Failed to create new hasher: %s", err)
	}

	return NewTrillianSigner(hasher, trillian.SignatureAlgorithm_RSA, mock)
}

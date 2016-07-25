package ct

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/fixchain"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/examples/ct/testonly"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var fixedTime time.Time = time.Date(2016, 21, 7, 12, 15, 23, 0, time.UTC)

// Public key for Google Testtube log, taken from CT Github repository
const ctTesttubePublicKey string = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEw8i8S7qiGEs9NXv0ZJFh6uuOmR2Q
7dPprzk9XNNGkUXjzqx2SDvRfiwKYwBljfWujozHESVPQyydGaHhkaSz/g==
-----END PUBLIC KEY-----`

// Log ID for testtube log, from known logs page
const ctTesttubeLogID string = "sMyD5aX5fWuvfAnMKEkEhyrH6IsTLGNQt8b9JuFsbHc="

// Log ID for a dummy log public key of "key" supplied by mock
const ctMockLogID string = "LHDhK3oGRvkiefQnx7OOczTY5Tic/xZ6HcMOc/gmtoM="

// This test uses the testtube key rather than our test key so we can verify the
// result easily
func TestGetCTLogID(t *testing.T) {
	km := crypto.NewPEMKeyManager()
	err := km.LoadPublicKey(ctTesttubePublicKey)
	assert.NoError(t, err, "unexpected error loading public key")

	expected := ctTesttubeLogID
	got, err := GetCTLogID(km)
	assert.NoError(t, err, "error getting logid")

	got64 := base64.StdEncoding.EncodeToString(got[:])

	if expected != got64 {
		t.Fatalf("expected logID: %s but got: %s", expected, got64)
	}
}

func TestGetCTLogIDNotLoaded(t *testing.T) {
	km := crypto.NewPEMKeyManager()

	_, err := GetCTLogID(km)

	assert.Error(t, err, "expected error when no key loaded")
}

func TestSignV1SCTForCertificate(t *testing.T) {
	cert, err := fixchain.CertificateFromPEM(testonly.LeafSignedByFakeIntermediateCertPem)

	if err != nil {
		t.Fatalf("failed to set up test cert: %v", err)
	}

	km := setupMockKeyManager([]byte{0xd1, 0x66, 0x49, 0xc7, 0xbb, 0x48, 0xe7, 0x32, 0xa9, 0x71, 0xc3, 0x1b, 0x26, 0xf6, 0x5c, 0x26, 0x85, 0xd3, 0xc, 0xed, 0x22, 0x48, 0xc4, 0xd4, 0xdb, 0xaa, 0xee, 0x9d, 0x44, 0xf4, 0xc1, 0x6f})

	got, err := SignV1SCTForCertificate(km, cert, fixedTime)

	if err != nil {
		t.Fatalf("create sct for cert failed", err)
	}

	logID, err := base64.StdEncoding.DecodeString(ctMockLogID)

	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{SCTVersion: 0,
		LogID:      ct.SHA256Hash(idArray),
		Timestamp:  1504786523000000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			HashAlgorithm:      ct.SHA256,
			SignatureAlgorithm: ct.RSA,
			Signature:          []byte("signed")}}

	assert.Equal(t, expected, got, "mismatched SCT (cert)")
}

func TestSignV1SCTForPrecertificate(t *testing.T) {
	cert, err := fixchain.CertificateFromPEM(testonly.CTTestPreCert)
	_, ok := err.(x509.NonFatalErrors)

	if err != nil && !ok {
		t.Fatalf("failed to set up test precert: %v", err)
	}

	km := setupMockKeyManager([]byte{0xfd, 0x5c, 0x80, 0xd6, 0x5c, 0xdc, 0xee, 0xc, 0x6e, 0xc8, 0xc3, 0xfb, 0xb3, 0xe6, 0x4b, 0xc9, 0x1e, 0x1e, 0x9a, 0xf5, 0x18, 0x99, 0xeb, 0x86, 0x68, 0x0, 0xb5, 0xc, 0x36, 0x30, 0xc9, 0xf})

	got, err := SignV1SCTForPrecertificate(km, cert, fixedTime)

	if err != nil {
		t.Fatalf("create sct for precert failed", err)
	}

	logID, err := base64.StdEncoding.DecodeString(ctMockLogID)

	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{SCTVersion: 0,
		LogID:      ct.SHA256Hash(idArray),
		Timestamp:  1504786523000000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{HashAlgorithm: ct.SHA256,
			SignatureAlgorithm: ct.RSA,
			Signature:          []byte("signed")}}

	assert.Equal(t, expected, got, "mismatched SCT (precert")
}

func setupMockKeyManager(toSign []byte) crypto.KeyManager {
	hasher := trillian.NewSHA256()
	mockKeyManager := new(crypto.MockKeyManager)
	mockSigner := new(crypto.MockSigner)
	mockSigner.On("Sign", mock.MatchedBy(
		func(other io.Reader) bool {
			return true
		}), toSign, hasher).Return([]byte("signed"), nil)
	mockKeyManager.On("Signer").Return(mockSigner, nil)
	mockKeyManager.On("GetRawPublicKey").Return([]byte("key"), nil)

	return mockKeyManager
}

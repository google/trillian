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

// Test data taken from CT repository cpp/log/test_signer.cc

// Some time in September 2012.
const defaultSCTTimestamp uint64 = 1348589665525

const defaultCertSCTSignatureB64 string = "3046022100d3f7690e7ee80d9988a54a3821056393e9eb0c686ad67fbae3686c888fb1a3ce022100f9a51c6065bbba7ad7116a31bea1c31dbed6a921e1df02e4b403757fae3254ae"
const defaultPrecertSCTSignatureB64 string = "304502206f247c7d1abe2b8f6c4530f99474f9ebe90629d21f76616389336f177ed7a7d00221009d3a60c2b407ab5a725a692fc79d0d301d6da61baec43175ed07514c535f1120"

const ecP256PrivateKeyPEM string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIG8QAquNnarN6Ik2cMIZtPBugh9wNRe0e309MCmDfBGuoAoGCCqGSM49
AwEHoUQDQgAES0AfBkjr7b8b19p5Gk8plSAN16wWXZyhYsH6FMCEUK60t7pem/ck
oPX8hupuaiJzJS0ZQ0SEoJGlFxkUFwft5g==
-----END EC PRIVATE KEY-----`

const ecP256PublicKeyPEM string = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAES0AfBkjr7b8b19p5Gk8plSAN16wW
XZyhYsH6FMCEUK60t7pem/ckoPX8hupuaiJzJS0ZQ0SEoJGlFxkUFwft5g==
-----END PUBLIC KEY-----`

const keyIDB64 string = "b69d879e3f2c4402556dcda2f6b2e02ff6b6df4789c53000e14f4b125ae847aa"

// end of CT test data

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

	km := setupMockKeyManager([]byte{0x5, 0x62, 0x4f, 0xb4, 0x9e, 0x32, 0x14, 0xb6, 0xc, 0xb8, 0x51, 0x28, 0x23, 0x93, 0x2c, 0x7a, 0x3d, 0x80, 0x93, 0x5f, 0xcd, 0x76, 0xef, 0x91, 0x6a, 0xaf, 0x1b, 0x8c, 0xe8, 0xb5, 0x2, 0xb5})

	_, got, err := SignV1SCTForCertificate(km, cert, fixedTime)

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

	km := setupMockKeyManager([]byte{0x76, 0x2b, 0xa0, 0x63, 0x44, 0x5f, 0x52, 0x32, 0xb0, 0xd5, 0x9f, 0xd4, 0xab, 0x62, 0x55, 0x1e, 0xe8, 0xc0, 0xce, 0xe5, 0x20, 0x74, 0xba, 0x14, 0xd6, 0x88, 0xee, 0x7a, 0x79, 0x96, 0xee, 0xb9})

	_, got, err := SignV1SCTForPrecertificate(km, cert, fixedTime)

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

// Creates a mock key manager for use in interaction tests
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

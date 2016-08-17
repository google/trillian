package ct

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"github.com/google/certificate-transparency/go"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
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

func TestSerializeCTLogEntry(t *testing.T) {
	ts := ct.TimestampedEntry{
		Timestamp:  12345,
		EntryType:  ct.X509LogEntryType,
		X509Entry:  ct.ASN1Cert([]byte{0x10, 0x11, 0x12, 0x13, 0x20, 0x21, 0x22, 0x23}),
		Extensions: ct.CTExtensions{}}
	leaf := ct.MerkleTreeLeaf{LeafType: ct.TimestampedEntryLeafType, Version: ct.V1, TimestampedEntry: ts}

	for chainLength := 1; chainLength < 10; chainLength++ {
		chain := createCertChain(chainLength)

		var buff bytes.Buffer
		w := bufio.NewWriter(&buff)

		logEntry := CTLogEntry{Leaf: leaf, Chain: chain}
		err := logEntry.Serialize(w)

		if err != nil {
			t.Fatalf("failed to serialize log entry: %v", err)
		}

		w.Flush()
		r := bufio.NewReader(&buff)

		var logEntry2 CTLogEntry
		err = logEntry2.Deserialize(r)

		if err != nil {
			t.Fatalf("failed to deserialize log entry: %v", err)
		}

		assert.Equal(t, logEntry, logEntry2, "log entry mismatch after serialization roundtrip")
	}
}

// Creates a mock key manager for use in interaction tests
func setupMockKeyManager(toSign []byte) *crypto.MockKeyManager {
	mockKeyManager := setupMockKeyManagerForSth(toSign)
	mockKeyManager.On("GetRawPublicKey").Return([]byte("key"), nil)

	return mockKeyManager
}

// As above but we don't expect the call for a public key as we don't need it for an STH
func setupMockKeyManagerForSth(toSign []byte) *crypto.MockKeyManager {
	hasher := trillian.NewSHA256()
	mockKeyManager := new(crypto.MockKeyManager)
	mockSigner := new(crypto.MockSigner)
	mockSigner.On("Sign", mock.MatchedBy(
		func(other io.Reader) bool {
			return true
		}), toSign, hasher).Return([]byte("signed"), nil)
	mockKeyManager.On("Signer").Return(mockSigner, nil)

	return mockKeyManager
}

// Creates a dummy cert chain
func createCertChain(numCerts int) []ct.ASN1Cert {
	chain := make([]ct.ASN1Cert, 0, numCerts)

	for c := 0; c < numCerts; c++ {
		certBytes := make([]byte, c+2)

		for i := 0; i < c+2; i++ {
			certBytes[i] = byte(c)
		}

		chain = append(chain, ct.ASN1Cert(certBytes))
	}

	return chain
}

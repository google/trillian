package ct

import (
	"encoding/base64"
	"testing"

	"github.com/google/trillian/crypto"
	"github.com/stretchr/testify/assert"
)

// Public key for Google Testtube log, taken from CT Github repository
const ctTesttubePublicKey string = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEw8i8S7qiGEs9NXv0ZJFh6uuOmR2Q
7dPprzk9XNNGkUXjzqx2SDvRfiwKYwBljfWujozHESVPQyydGaHhkaSz/g==
-----END PUBLIC KEY-----`

// Log ID for testtube log, from known logs page
const ctTesttubeLogID string = "sMyD5aX5fWuvfAnMKEkEhyrH6IsTLGNQt8b9JuFsbHc="

func TestGetCTLogID(t *testing.T) {
	km := crypto.NewPEMKeyManager()
	err := km.LoadPublicKey(ctTesttubePublicKey)
	assert.NoError(t, err, "unexpected error loading public key")

	expected := ctTesttubeLogID
	got, err := GetCTLogID(km)
	assert.NoError(t, err, "error geting logid")

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
}

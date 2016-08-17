package testonly

import (
	"testing"
	"encoding/hex"
	"encoding/pem"

	"github.com/google/certificate-transparency/go/x509"
)

// PemToCert decodes a string in PEM format assumed to contain certificate data and
// returns a pointer to an X509.Certificate built by parsing it. Non fatal errors
// in parsing are ignored.
func PemToCert(t *testing.T, pemData string) *x509.Certificate {
	bytes, rest := pem.Decode([]byte(pemData))

	if len(rest) > 0 {
		t.Fatalf("Extra data after PEM: %v", rest)
		return nil
	}

	cert, err := x509.ParseCertificate(bytes.Bytes)

	if err != nil {
		_, ok := err.(x509.NonFatalErrors)

		if !ok {
			t.Fatal(err)
			return nil
		}
	}

	return cert
}

// MustHexDecode decodes its input string from hex and panics if this fails
func MustHexDecode(b string) []byte {
	r, err := hex.DecodeString(b)
	if err != nil {
		panic(err)
	}
	return r
}

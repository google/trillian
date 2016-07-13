package ct_fe

import (
	"encoding/pem"
	"testing"

	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/google/trillian/testonly"
	"github.com/stretchr/testify/assert"
)

func TestIsPrecertificate(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)

	isPrecert, err := IsPrecertificate(cert)

	assert.Nil(t, err, "Expected no error from precert check")
	assert.True(t, isPrecert, "Valid precert not recognized")

	// Wipe all the extensions and try again
	cert.Extensions = cert.Extensions[:0]
	isPrecert, err = IsPrecertificate(cert)

	assert.Nil(t, err, "Expected no error from precert check")
	assert.False(t, isPrecert, "Non precert misclassified")
}

func TestIsPrecertificateNormalCert(t *testing.T) {
	cert := pemToCert(t, testonly.CACertPEM)

	isPrecert, err := IsPrecertificate(cert)

	assert.Nil(t, err, "Expected no error from precert check")
	assert.False(t, isPrecert, "Valid precert not recognized")
}

func TestIsPrecertificateInvalidNonCriticalExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not marked as critical
	ext := pkix.Extension{Id: ctPoisonExtensionOid, Critical: false, Value: asn1NullBytes}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	assert.Error(t, err, "Incorrectly accepted non critical CT extension")
}

func TestIsPrecertificateInvalidBytesInExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not asn.1 null
	ext := pkix.Extension{Id: ctPoisonExtensionOid, Critical: false, Value: []byte{0x42, 0x42, 0x42}}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	assert.Error(t, err, "Incorrectly accepted invalid CT extension")
}

func pemToCert(t *testing.T, pemData string) *x509.Certificate {
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

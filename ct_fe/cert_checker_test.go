package ct_fe

import (
	"testing"
	"encoding/pem"

	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/stretchr/testify/assert"
)

const validPrecertPEM string = `
-----BEGIN CERTIFICATE-----
MIIC3zCCAkigAwIBAgIBBzANBgkqhkiG9w0BAQUFADBVMQswCQYDVQQGEwJHQjEk
MCIGA1UEChMbQ2VydGlmaWNhdGUgVHJhbnNwYXJlbmN5IENBMQ4wDAYDVQQIEwVX
YWxlczEQMA4GA1UEBxMHRXJ3IFdlbjAeFw0xMjA2MDEwMDAwMDBaFw0yMjA2MDEw
MDAwMDBaMFIxCzAJBgNVBAYTAkdCMSEwHwYDVQQKExhDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kxDjAMBgNVBAgTBVdhbGVzMRAwDgYDVQQHEwdFcncgV2VuMIGfMA0G
CSqGSIb3DQEBAQUAA4GNADCBiQKBgQC+75jnwmh3rjhfdTJaDB0ym+3xj6r015a/
BH634c4VyVui+A7kWL19uG+KSyUhkaeb1wDDjpwDibRc1NyaEgqyHgy0HNDnKAWk
EM2cW9tdSSdyba8XEPYBhzd+olsaHjnu0LiBGdwVTcaPfajjDK8VijPmyVCfSgWw
FAn/Xdh+tQIDAQABo4HBMIG+MB0GA1UdDgQWBBQgMVQa8lwF/9hli2hDeU9ekDb3
tDB9BgNVHSMEdjB0gBRfnYgNyHPmVNT4DdjmsMEktEfDVaFZpFcwVTELMAkGA1UE
BhMCR0IxJDAiBgNVBAoTG0NlcnRpZmljYXRlIFRyYW5zcGFyZW5jeSBDQTEOMAwG
A1UECBMFV2FsZXMxEDAOBgNVBAcTB0VydyBXZW6CAQAwCQYDVR0TBAIwADATBgor
BgEEAdZ5AgQDAQH/BAIFADANBgkqhkiG9w0BAQUFAAOBgQACocOeAVr1Tf8CPDNg
h1//NDdVLx8JAb3CVDFfM3K3I/sV+87MTfRxoM5NjFRlXYSHl/soHj36u0YtLGhL
BW/qe2O0cP8WbjLURgY1s9K8bagkmyYw5x/DTwjyPdTuIo+PdPY9eGMR3QpYEUBf
kGzKLC0+6/yBmWTr2M98CIY/vg==
-----END CERTIFICATE-----
`

func TestIsPrecertificate(t *testing.T) {
	cert := pemToCert(t, validPrecertPEM)

	isPrecert, err := IsPrecertificate(cert)

	assert.Nil(t, err, "Expected no error from precert check")
	assert.True(t, isPrecert, "Valid precert not recognized")

	// Wipe all the extensions and try again
	cert.Extensions = cert.Extensions[:0]
	isPrecert, err = IsPrecertificate(cert)

	assert.Nil(t, err, "Expected no error from precert check")
	assert.False(t, isPrecert, "Non precert misclassified")
}

func TestIsPrecertificateInvalidNonCriticalExtension(t *testing.T) {
	cert := pemToCert(t, validPrecertPEM)
	// Invalid because it's not marked as critical
	ext := pkix.Extension{Id: ctPoisonExtensionOid, Critical: false, Value: asn1NullBytes}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	assert.Error(t, err, "Incorrectly accepted non critical CT extension")
}

func TestIsPrecertificateInvalidBytesInExtension(t *testing.T) {
	cert := pemToCert(t, validPrecertPEM)
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
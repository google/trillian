package ct

import (
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/google/trillian/examples/ct/testonly"
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
	assert.False(t, isPrecert, "Non precert misclassified")
}

func TestIsPrecertificateInvalidNonCriticalExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not marked as critical
	ext := pkix.Extension{Id: ctPoisonExtensionOID, Critical: false, Value: asn1NullBytes}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	assert.Error(t, err, "incorrectly accepted non critical CT extension")
}

func TestIsPrecertificateInvalidBytesInExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not asn.1 null
	ext := pkix.Extension{Id: ctPoisonExtensionOID, Critical: false, Value: []byte{0x42, 0x42, 0x42}}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	assert.Error(t, err, "incorrectly accepted invalid CT extension")
}

func TestCertCheckerInvalidChainAccepted(t *testing.T) {
	// This shouldn't validate as it's missing the intermediate cert
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPem}
	jsonChain := pemsToJsonChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	assert.True(t, trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPem)), "failed to load fake root")

	_, err := ValidateChain(jsonChain, *trustedRoots)

	assert.Error(t, err, "verification accepted an invalid chain (missing intermediate)")
}

func TestCertCheckerInvalidChainRejectedOrdering(t *testing.T) {
	// This chain shouldn't validate because the order of presentation is wrong
	chainPem := []string{testonly.FakeIntermediateCertPem, testonly.LeafSignedByFakeIntermediateCertPem}
	jsonChain := pemsToJsonChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	assert.True(t, trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPem)), "failed to load fake root")

	_, err := ValidateChain(jsonChain, *trustedRoots)

	assert.Error(t, err, "verification accepted an invalid chain (ordering)")
}

func TestCertCheckerInvalidChainRejectedBadChain(t *testing.T) {
	// This chain shouldn't validate because the chain contains unrelated certs
	chainPem := []string{testonly.FakeIntermediateCertPem, testonly.TestCertPEM}
	jsonChain := pemsToJsonChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	assert.True(t, trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPem)), "failed to load fake root")

	_, err := ValidateChain(jsonChain, *trustedRoots)

	assert.Error(t, err, "verification accepted an invalid chain (unrelated)")
}

func TestCertCheckerInvalidChainRejectedBadChainUnrelatedAppended(t *testing.T) {
	// This chain shouldn't validate because the otherwise valid chain contains an unrelated cert
	// at the end
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPem, testonly.FakeIntermediateCertPem, testonly.TestCertPEM}
	jsonChain := pemsToJsonChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	assert.True(t, trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPem)), "failed to load fake root")

	_, err := ValidateChain(jsonChain, *trustedRoots)

	assert.Error(t, err, "verification accepted an invalid chain (unrelated at end)")
}

func TestCertCheckerValidChainAccepted(t *testing.T) {
	// This chain should validate up to the fake root CA
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPem, testonly.FakeIntermediateCertPem}
	jsonChain := pemsToJsonChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	assert.True(t, trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPem)), "failed to load fake root")

	validPath, err := ValidateChain(jsonChain, *trustedRoots)

	assert.NoError(t, err, "unexpected error verifying valid chain")
	assert.Equal(t, 2, len(validPath), "expected valid path of length 2")
}

// Builds a chain of base64 encoded certs as if they'd been submitted to a handler.
// Note: ordering is important
func pemsToJsonChain(t *testing.T, pemCerts []string) []string {
	chain := []string{}

	for _, pem := range pemCerts {
		cert := pemToCert(t, pem)
		chain = append(chain, base64.StdEncoding.EncodeToString(cert.Raw))
	}

	return chain
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

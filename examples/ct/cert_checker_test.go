package ct

import (
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/google/trillian/examples/ct/testonly"
)

func TestIsPrecertificate(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)

	isPrecert, err := IsPrecertificate(cert)
	if err != nil {
		t.Fatalf("Unexpected error from precert check %v", err)
	}
	if !isPrecert {
		t.Fatal("Valid precert not recognized")
	}

	// Wipe all the extensions and try again
	cert.Extensions = cert.Extensions[:0]
	isPrecert, err = IsPrecertificate(cert)

	if err != nil {
		t.Fatalf("Unexpected error from precert check %v", err)
	}
	if isPrecert {
		t.Fatal("Non precert misclassified")
	}
}

func TestIsPrecertificateNormalCert(t *testing.T) {
	cert := pemToCert(t, testonly.CACertPEM)

	isPrecert, err := IsPrecertificate(cert)

	if err != nil {
		t.Fatalf("Unexpected error from precert check %v", err)
	}
	if isPrecert {
		t.Fatal("Non precert misclassified")
	}
}

func TestIsPrecertificateInvalidNonCriticalExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not marked as critical
	ext := pkix.Extension{Id: ctPoisonExtensionOID, Critical: false, Value: asn1NullBytes}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)
	if err == nil {
		t.Fatal("incorrectly accepted non critical CT extension")
	}
}

func TestIsPrecertificateInvalidBytesInExtension(t *testing.T) {
	cert := pemToCert(t, testonly.PrecertPEMValid)
	// Invalid because it's not asn.1 null
	ext := pkix.Extension{Id: ctPoisonExtensionOID, Critical: false, Value: []byte{0x42, 0x42, 0x42}}

	cert.Extensions = []pkix.Extension{ext}
	_, err := IsPrecertificate(cert)

	if err == nil {
		t.Fatal("incorrectly accepted invalid CT extension")
	}
}

func TestCertCheckerInvalidChainAccepted(t *testing.T) {
	// This shouldn't validate as it's missing the intermediate cert
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPEM}
	jsonChain := pemsToJSONChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	if !trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}

	_, err := ValidateChain(jsonChain, *trustedRoots)

	if err == nil {
		t.Fatal("verification accepted an invalid chain (missing intermediate)")
	}
}

func TestCertCheckerInvalidChainRejectedOrdering(t *testing.T) {
	// This chain shouldn't validate because the order of presentation is wrong
	chainPem := []string{testonly.FakeIntermediateCertPEM, testonly.LeafSignedByFakeIntermediateCertPEM}
	jsonChain := pemsToJSONChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	if !trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}

	_, err := ValidateChain(jsonChain, *trustedRoots)

	if err == nil {
		t.Fatal("verification accepted an invalid chain (ordering)")
	}
}

func TestCertCheckerInvalidChainRejectedBadChain(t *testing.T) {
	// This chain shouldn't validate because the chain contains unrelated certs
	chainPem := []string{testonly.FakeIntermediateCertPEM, testonly.TestCertPEM}
	jsonChain := pemsToJSONChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	if !trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}

	_, err := ValidateChain(jsonChain, *trustedRoots)

	if err == nil {
		t.Fatal("verification accepted an invalid chain (unrelated)")
	}
}

func TestCertCheckerInvalidChainRejectedBadChainUnrelatedAppended(t *testing.T) {
	// This chain shouldn't validate because the otherwise valid chain contains an unrelated cert
	// at the end
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM, testonly.TestCertPEM}
	jsonChain := pemsToJSONChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	if !trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}

	_, err := ValidateChain(jsonChain, *trustedRoots)

	if err == nil {
		t.Fatal("verification accepted an invalid chain (unrelated at end)")
	}
}

func TestCertCheckerValidChainAccepted(t *testing.T) {
	// This chain should validate up to the fake root CA
	chainPem := []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM}
	jsonChain := pemsToJSONChain(t, chainPem)
	trustedRoots := NewPEMCertPool()

	if !trustedRoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}

	validPath, err := ValidateChain(jsonChain, *trustedRoots)

	if err != nil {
		t.Fatalf("unexpected error verifying valid chain %v", err)
	}
	if got, want := len(validPath), 2; got != want {
		t.Fatalf(" got path of len %d, but expected length %d", got, want)
	}
}

// Builds a chain of base64 encoded certs as if they'd been submitted to a handler.
// Note: ordering is important
func pemsToJSONChain(t *testing.T, pemCerts []string) []string {
	chain := []string{}

	for _, pemCert := range pemCerts {
		cert := pemToCert(t, pemCert)
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

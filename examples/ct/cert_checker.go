package ct

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/certificate-transparency/go/asn1"
	"github.com/google/certificate-transparency/go/x509"
)

// OID of the non-critical extension used to mark pre-certificates, defined in RFC 6962
var ctPoisonExtensionOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 3}

// Byte representation of ASN.1 NULL.
var asn1NullBytes = []byte{0x05, 0x00}

// IsPrecertificate tests if a certificate is a pre-certificate as defined in CT.
// An error is returned if the CT extension is present but is not ASN.1 NULL as defined
// by the spec.
func IsPrecertificate(cert *x509.Certificate) (bool, error) {
	for _, ext := range cert.Extensions {
		if ctPoisonExtensionOID.Equal(ext.Id) {
			if !ext.Critical || !bytes.Equal(asn1NullBytes, ext.Value) {
				return false, fmt.Errorf("CT poison ext is not critical or invalid: %v", ext)
			}

			return true, nil
		}
	}

	return false, nil
}

// ValidateChain takes the certificate chain as it was parsed from a JSON request. Ensures all
// elements in the chain decode as X.509 certificates. Ensures that there is a valid path from the
// end entity certificate in the chain to a trusted root cert, possibly using the intermediates
// supplied in the chain. Then applies the RFC requirement that the path must involve all
// the submitted chain in the order of submission.
func ValidateChain(jsonChain []string, trustedRoots PEMCertPool) ([]*x509.Certificate, error) {
	// First decode the base 64 certs and make sure they parse as X.509
	chain := make([]*x509.Certificate, 0, len(jsonChain))
	intermediatePool := NewPEMCertPool()

	for i, certB64 := range jsonChain {
		certBytes, err := base64.StdEncoding.DecodeString(certB64)

		if err != nil {
			return nil, err
		}

		cert, err := x509.ParseCertificate(certBytes)

		if err != nil {
			_, ok := err.(x509.NonFatalErrors)

			if !ok {
				return nil, err
			}
		}

		chain = append(chain, cert)

		// All but the first cert form part of the intermediate pool
		if i > 0 {
			intermediatePool.AddCert(cert)
		}
	}

	// We can now do the verify
	// TODO(Martin2112): Check this is the correct key usage value to use
	verifyOpts := x509.VerifyOptions{
		Roots:             trustedRoots.CertPool(),
		Intermediates:     intermediatePool.CertPool(),
		DisableTimeChecks: true,
		KeyUsages:         []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}

	// We don't want failures from Verify due to unknown critical extensions,
	// so clear them out.
	chain[0].UnhandledCriticalExtensions = nil
	chains, err := chain[0].Verify(verifyOpts)

	if err != nil {
		return nil, err
	}

	if len(chains) == 0 {
		return nil, errors.New("no path to root found when trying to validate chains")
	}

	// Verify might have found multiple paths to roots. Now we check that we have a path that
	// uses all the certs in the order they were submitted so as to comply with RFC 6962
	// requirements detailed in Section 3.1.
	for _, verifiedChain := range chains {
		// The verified chain includes a root, which we don't need to include in the comparison
		chainMinusRoot := verifiedChain[:len(verifiedChain)-1]

		if len(chainMinusRoot) != len(chain) {
			continue
		}

		for i, certInChain := range chainMinusRoot {
			if certInChain != chain[i] {
				continue
			}
		}

		return chainMinusRoot, nil
	}

	return nil, errors.New("no RFC compliant path to root found when trying to validate chain")
}

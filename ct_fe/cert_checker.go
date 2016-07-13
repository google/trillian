package ct_fe

import (
	"encoding/base64"
	"errors"

	"github.com/google/trillian/crypto"
	"github.com/google/certificate-transparency/go/x509"
)

// ValidateChain takes the certificate chain as it was parsed from a JSON request. Ensures all
// elements in the chain decode as X.509 certificates. Ensures that there is a valid path from the
// end entity certificate in the chain to a trusted root cert, possibly using the intermediates
// supplied in the chain.
func ValidateChain(jsonChain []string, trustedRoots crypto.PEMCertPool) ([]*x509.Certificate, error) {
	// First decode the base 64 certs and make sure they parse as X.509
	chain := make([]*x509.Certificate, 0, len(jsonChain))
	intermediatePool := crypto.NewPEMCertPool()

	for i, certB64 := range jsonChain {
		certBytes, err := base64.StdEncoding.DecodeString(certB64)

		if err != nil {
			return nil, err
		}

		cert, err := x509.ParseCertificate(certBytes)

		if err != nil {
			return nil, err
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
		Roots: trustedRoots.CertPool(),
		Intermediates: intermediatePool.CertPool(),
		DisableTimeChecks: true,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}

	chains, err := chain[0].Verify(verifyOpts)

	if err != nil {
		return nil, err
	}

	if len(chains) == 0 {
		return nil, errors.New("No path to root found when trying to validate chains")
	}

	// Verify might have found multiple paths to roots. Now we check that we have a path that
	// uses all the certs in the order they were submitted so as to comply with RFC 6962
	// requirements detailed in Section 3.1.
	for _, verifiedChain := range chains {
		// The verified chain includes a root, which we don't need to include in the comparison
		chainMinusRoot := verifiedChain[:len(verifiedChain) - 1]

		if len(chainMinusRoot) != len(chain) {
			continue
		}

		for i, certInChain := range chainMinusRoot {
			if certInChain != chain[i] {
				continue
			}
		}

		return chain, nil
	}

	return nil, errors.New("Invalid path to root found when trying to validate chain")
}

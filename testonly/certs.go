package testonly

import (
	"encoding/pem"

	ct "github.com/google/certificate-transparency/go"
)

// CertsFromPEM loads X.509 certificates from the provided PEM-encoded data.
func CertsFromPEM(data []byte) []ct.ASN1Cert {
	var chain []ct.ASN1Cert
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			chain = append(chain, ct.ASN1Cert{Data: block.Bytes})
		}
	}
	return chain
}

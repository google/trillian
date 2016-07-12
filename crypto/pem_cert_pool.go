package crypto

import (
	"encoding/pem"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency/go/x509"
	"crypto/sha1"
)

// PEMCertPool is a wrapper / extension to x509.CertPool. It allows us to access the
// raw certs, which we need to serve get-roots request and has stricter handling on loading
// certs into the pool. CertPool ignores errors if at least one cert loads correctly but
// PEMCertPool requires all certs to load.
type PEMCertPool struct {
	// maps from sha-1 to certificate, used for dup detection
	fingerprintToCertMap map[[sha1.Size]byte]x509.Certificate
	rawCerts             [][]byte
	certPool             *x509.CertPool
}

// Creates a new instance of PEMCertPool containing no certificates.
func NewPEMCertPool() *PEMCertPool {
	return &PEMCertPool{fingerprintToCertMap: make(map[[sha1.Size]byte]x509.Certificate), certPool: x509.NewCertPool()}
}

// AddCert adds a certificate to a pool. Uses fingerprint to weed out duplicates.
// cert must not be nil.
func (p *PEMCertPool) AddCert(cert *x509.Certificate) {
	fingerprint := sha1.Sum(cert.Raw)
	_, ok := p.fingerprintToCertMap[fingerprint]

	if !ok {
		p.fingerprintToCertMap[fingerprint] = *cert
		p.certPool.AddCert(cert)
		p.rawCerts = append(p.rawCerts, cert.Raw)
	}
}

// AppendCertsFromPEM adds certs to the pool from a byte slice assumed to contain PEM encoded data.
// Skips over non certificate blocks in the data. Returns true if all certificates in the
// data were parsed and added to the pool successfully and at least one certificate was found.
func (p *PEMCertPool) AppendCertsFromPEM(pemCerts []byte) (ok bool) {
	for len(pemCerts) > 0 {
		var block *pem.Block
		block, pemCerts = pem.Decode(pemCerts)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			glog.Warningf("error parsing PEM certificate: %v", err)
			return false
		}

		p.AddCert(cert)
		ok = true
	}

	return
}

// Subjects returns a list of the DER-encoded subjects of all of the certificates in the pool.
func (p *PEMCertPool) Subjects() (res [][]byte) {
	return p.certPool.Subjects()
}
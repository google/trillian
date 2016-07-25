package ct

// Code to handle encoding / decoding various data structures used in RFC 6962.

import (
	"crypto/sha256"
	"time"

	"github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
)

const millisPerNano int64 = 1000

// GetCTKeyID takes the key manager for a log and returns the LogID. (see RFC 6962 S3.2)
// In CT V1 the log id is a hash of the public key.
func GetCTLogID(km crypto.KeyManager) ([sha256.Size]byte, error) {
	key, err := km.GetRawPublicKey()

	if err != nil {
		return [sha256.Size]byte{}, err
	}

	return sha256.Sum256(key), nil
}

func signSCT(km crypto.KeyManager, t time.Time, sctData []byte) (ct.SignedCertificateTimestamp, error) {
	signer, err := km.Signer()
	if err != nil {
		return ct.SignedCertificateTimestamp{}, err
	}

	// TODO(Martin2112): Algorithms shouldn't be hardcoded here, needs more work in key manager
	trillianSigner := crypto.NewTrillianSigner(trillian.NewSHA256(), trillian.SignatureAlgorithm_RSA, signer)

	signature, err := trillianSigner.Sign(sctData)

	if err != nil {
		return ct.SignedCertificateTimestamp{}, err
	}

	digitallySigned := ct.DigitallySigned{
		HashAlgorithm:      ct.SHA256,
		SignatureAlgorithm: ct.RSA,
		Signature:          signature.Signature}

	logID, err := GetCTLogID(km)

	if err != nil {
		return ct.SignedCertificateTimestamp{}, err
	}

	return ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		LogID:      logID,
		Timestamp:  uint64(t.UnixNano() / millisPerNano), // spec uses millisecond timestamps
		Extensions: ct.CTExtensions{},
		Signature:  digitallySigned}, nil
}

// CreateV1SCTForCertificate builds and signs a V1 CT SCT for a certificate using the key held
// by a key manager.
func SignV1SCTForCertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.SignedCertificateTimestamp, error) {
	return signSCT(km, t, cert.Raw)
}

// CreateV1SCTForPrecertificate builds and signs a V1 CT SCT for a pre-certificate using the key
// held by a key manager.
func SignV1SCTForPrecertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.SignedCertificateTimestamp, error) {
	// For precerts we need to extract the relevant data from the Certificate container.
	// This is only possible using our modified version of X.509.
	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	tbsBytes := make([]byte, 0, len(cert.RawTBSCertificate)+sha256.Size)
	tbsBytes = append(tbsBytes, keyHash[:]...)
	tbsBytes = append(tbsBytes, cert.RawTBSCertificate...)

	return signSCT(km, t, tbsBytes)
}

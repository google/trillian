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

func serializeAndSignSCT(km crypto.KeyManager, leaf ct.MerkleTreeLeaf, sctInput ct.SignedCertificateTimestamp, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Serialize SCT signature input to get the bytes that need to be signed
	res, err := ct.SerializeSCTSignatureInput(sctInput, ct.LogEntry{Leaf:leaf})

	if err != nil {
		return ct.MerkleTreeLeaf{}, ct.SignedCertificateTimestamp{}, err
	}

	// Create a complete SCT including signature
	sct, err := signSCT(km, t, res)

	return leaf, sct, err
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

// CreateV1SCTForCertificate creates a MerkleTreeLeaf and builds and signs a V1 CT SCT for a certificate
// using the key held by a key manager.
func SignV1SCTForCertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Temp SCT for input to the serializer
	sctInput := getSCTForSignatureInput(t)

	// Build up a MerkleTreeLeaf for the cert
	timestampedEntry := ct.TimestampedEntry{Timestamp:sctInput.Timestamp, EntryType:ct.X509LogEntryType, X509Entry:cert.Raw}
	leaf := ct.MerkleTreeLeaf{Version: ct.V1, LeafType:ct.TimestampedEntryLeafType, TimestampedEntry:timestampedEntry}

	return serializeAndSignSCT(km, leaf, sctInput, t)
}

// CreateV1SCTForPrecertificate builds and signs a V1 CT SCT for a pre-certificate using the key
// held by a key manager.
func SignV1SCTForPrecertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Temp SCT for input to the serializer
	sctInput := getSCTForSignatureInput(t)

	// Build up a LogEntry for the precert
	// For precerts we need to extract the relevant data from the Certificate container.
	// This is only possible using the CT specific modified version of X.509.
	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	precert := ct.PreCert{IssuerKeyHash: keyHash, TBSCertificate: cert.RawTBSCertificate}

	timestampedEntry := ct.TimestampedEntry{Timestamp:sctInput.Timestamp, EntryType:ct.PrecertLogEntryType, PrecertEntry:precert}
	leaf := ct.MerkleTreeLeaf{Version: ct.V1, LeafType:ct.TimestampedEntryLeafType, TimestampedEntry:timestampedEntry}

	return serializeAndSignSCT(km, leaf, sctInput, t)
}

func getSCTForSignatureInput(t time.Time) (ct.SignedCertificateTimestamp) {
	return ct.SignedCertificateTimestamp{
		SCTVersion:ct.V1,
		Timestamp:  uint64(t.UnixNano() / millisPerNano), // spec uses millisecond timestamps
		Extensions: ct.CTExtensions{}}
}
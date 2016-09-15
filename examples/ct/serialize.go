package ct

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
)

// SignV1TreeHead signs a tree head for CT. The input STH should have been built from a
// backend response and already checked for validity.
func signV1TreeHead(km crypto.KeyManager, sth *ct.SignedTreeHead) error {
	signer, err := km.Signer()

	if err != nil {
		return err
	}

	sthBytes, err := ct.SerializeSTHSignatureInput(*sth)

	if err != nil {
		return err
	}

	// TODO(Martin2112): Algorithms shouldn't be hardcoded here, needs more work in key manager
	trillianSigner := crypto.NewTrillianSigner(trillian.NewSHA256(), trillian.SignatureAlgorithm_RSA, signer)

	signature, err := trillianSigner.Sign(sthBytes)

	if err != nil {
		return err
	}

	sth.TreeHeadSignature = ct.DigitallySigned{
		HashAlgorithm:      ct.SHA256,
		SignatureAlgorithm: ct.RSA,
		Signature:          signature.Signature}

	return nil
}

// SignV1SCTForCertificate creates a MerkleTreeLeaf and builds and signs a V1 CT SCT for a certificate
// using the key held by a key manager.
func signV1SCTForCertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Temp SCT for input to the serializer
	sctInput := getSCTForSignatureInput(t)

	// Build up a MerkleTreeLeaf for the cert
	timestampedEntry := ct.TimestampedEntry{Timestamp: sctInput.Timestamp, EntryType: ct.X509LogEntryType, X509Entry: cert.Raw}
	leaf := ct.MerkleTreeLeaf{Version: ct.V1, LeafType: ct.TimestampedEntryLeafType, TimestampedEntry: timestampedEntry}

	return serializeAndSignSCT(km, leaf, sctInput, t)
}

// SignV1SCTForPrecertificate builds and signs a V1 CT SCT for a pre-certificate using the key
// held by a key manager.
func signV1SCTForPrecertificate(km crypto.KeyManager, cert *x509.Certificate, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Temp SCT for input to the serializer
	sctInput := getSCTForSignatureInput(t)

	// Build up a LogEntry for the precert
	// For precerts we need to extract the relevant data from the Certificate container.
	// This is only possible using the CT specific modified version of X.509.
	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	precert := ct.PreCert{IssuerKeyHash: keyHash, TBSCertificate: cert.RawTBSCertificate}

	timestampedEntry := ct.TimestampedEntry{Timestamp: sctInput.Timestamp, EntryType: ct.PrecertLogEntryType, PrecertEntry: precert}
	leaf := ct.MerkleTreeLeaf{Version: ct.V1, LeafType: ct.TimestampedEntryLeafType, TimestampedEntry: timestampedEntry}

	return serializeAndSignSCT(km, leaf, sctInput, t)
}

func serializeAndSignSCT(km crypto.KeyManager, leaf ct.MerkleTreeLeaf, sctInput ct.SignedCertificateTimestamp, t time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error) {
	// Serialize SCT signature input to get the bytes that need to be signed
	res, err := ct.SerializeSCTSignatureInput(sctInput, ct.LogEntry{Leaf: leaf})

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

func getSCTForSignatureInput(t time.Time) ct.SignedCertificateTimestamp {
	return ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		Timestamp:  uint64(t.UnixNano() / millisPerNano), // spec uses millisecond timestamps
		Extensions: ct.CTExtensions{}}
}

// WriteTimestampedEntry writes out a TimestampedEntry structure in the binary format defined
// by RFC 6962. The CT go code includes a deserializer but not a serializer so we might as
// well make this available.
func writeTimestampedEntry(w io.Writer, t ct.TimestampedEntry) error {
	if err := binary.Write(w, binary.BigEndian, &t.Timestamp); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, &t.EntryType); err != nil {
		return err
	}
	switch t.EntryType {
	case ct.X509LogEntryType:
		if err := writeVarBytes(w, t.X509Entry, ct.CertificateLengthBytes); err != nil {
			return err
		}
	case ct.PrecertLogEntryType:
		if err := binary.Write(w, binary.BigEndian, t.PrecertEntry.IssuerKeyHash); err != nil {
			return err
		}
		if err := writeVarBytes(w, t.PrecertEntry.TBSCertificate, ct.PreCertificateLengthBytes); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown EntryType: %d", t.EntryType)
	}

	return writeVarBytes(w, t.Extensions, ct.ExtensionsLengthBytes)
}

// WriteMerkleTreeLeaf writes a MerkleTreeLeaf in the binary format specified by RFC 6962.
// The CT go code includes a deserializer but not a serializer and we might as well make this
// available to other users.
func writeMerkleTreeLeaf(w io.Writer, l ct.MerkleTreeLeaf) error {
	if l.Version != ct.V1 {
		return fmt.Errorf("unknown Version: %d", l.Version)
	}

	if l.LeafType != ct.TimestampedEntryLeafType {
		return fmt.Errorf("unknown LeafType: %d", l.LeafType)
	}

	if err := binary.Write(w, binary.BigEndian, l.Version); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, l.LeafType); err != nil {
		return err
	}
	if err := writeTimestampedEntry(w, l.TimestampedEntry); err != nil {
		return err
	}

	return nil
}

// These came from the CT go code. Currently don't want to push changes upstream to make
// them visible but this is an option for the future.

func writeVarBytes(w io.Writer, value []byte, numLenBytes int) error {
	if err := writeUint(w, uint64(len(value)), numLenBytes); err != nil {
		return err
	}
	if _, err := w.Write(value); err != nil {
		return err
	}
	return nil
}

func writeUint(w io.Writer, value uint64, numBytes int) error {
	buf := make([]uint8, numBytes)
	for i := 0; i < numBytes; i++ {
		buf[numBytes-i-1] = uint8(value & 0xff)
		value >>= 8
	}
	if value != 0 {
		return errors.New("numBytes was insufficiently large to represent value")
	}
	if _, err := w.Write(buf); err != nil {
		return err
	}
	return nil
}

func readUint(r io.Reader, numBytes int) (uint64, error) {
	var l uint64
	for i := 0; i < numBytes; i++ {
		l <<= 8
		var t uint8
		if err := binary.Read(r, binary.BigEndian, &t); err != nil {
			return 0, err
		}
		l |= uint64(t)
	}
	return l, nil
}

// Reads a variable length array of bytes from |r|. |numLenBytes| specifies the
// number of (BigEndian) prefix-bytes which contain the length of the actual
// array data bytes that follow.
// Allocates an array to hold the contents and returns a slice view into it if
// the read was successful, or an error otherwise.
func readVarBytes(r io.Reader, numLenBytes int) ([]byte, error) {
	switch {
	case numLenBytes > 8:
		return nil, fmt.Errorf("numLenBytes too large (%d)", numLenBytes)
	case numLenBytes == 0:
		return nil, errors.New("numLenBytes should be > 0")
	}
	l, err := readUint(r, numLenBytes)
	if err != nil {
		return nil, err
	}
	data := make([]byte, l)
	n, err := r.Read(data)
	if err != nil {
		return nil, err
	}
	if n != int(l) {
		return nil, fmt.Errorf("short read: expected %d but got %d", l, n)
	}
	return data, nil
}

// End of code from CT repository

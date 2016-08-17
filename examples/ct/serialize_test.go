package ct

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/google/certificate-transparency/go/fixchain"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian/examples/ct/testonly"
	"github.com/stretchr/testify/assert"
	"github.com/google/certificate-transparency/go"
)

func TestSignV1SCTForCertificate(t *testing.T) {
	cert, err := fixchain.CertificateFromPEM(testonly.LeafSignedByFakeIntermediateCertPem)

	if err != nil {
		t.Fatalf("failed to set up test cert: %v", err)
	}

	km := setupMockKeyManager([]byte{0x5, 0x62, 0x4f, 0xb4, 0x9e, 0x32, 0x14, 0xb6, 0xc, 0xb8, 0x51, 0x28, 0x23, 0x93, 0x2c, 0x7a, 0x3d, 0x80, 0x93, 0x5f, 0xcd, 0x76, 0xef, 0x91, 0x6a, 0xaf, 0x1b, 0x8c, 0xe8, 0xb5, 0x2, 0xb5})

	leaf, got, err := signV1SCTForCertificate(km, cert, fixedTime)

	if err != nil {
		t.Fatalf("create sct for cert failed", err)
	}

	logID, err := base64.StdEncoding.DecodeString(ctMockLogID)

	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{SCTVersion: 0,
		LogID:      ct.SHA256Hash(idArray),
		Timestamp:  1504786523000000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			HashAlgorithm:      ct.SHA256,
			SignatureAlgorithm: ct.RSA,
			Signature:          []byte("signed")}}

	assert.Equal(t, expected, got, "mismatched SCT (cert)")

	// Additional checks that the MerkleTreeLeaf we built is correct
	assert.Equal(t, ct.V1, leaf.Version, "expected a v1 leaf")
	assert.Equal(t, ct.TimestampedEntryLeafType, leaf.LeafType, "expected a timestamped entry type")
	assert.Equal(t, ct.X509LogEntryType, leaf.TimestampedEntry.EntryType, "expected x509 entry type")
	assert.Equal(t, got.Timestamp, leaf.TimestampedEntry.Timestamp, "entry / sct timestamp mismatch")
	assert.Equal(t, ct.ASN1Cert(cert.Raw), leaf.TimestampedEntry.X509Entry, "cert bytes mismatch")
}

func TestSignV1SCTForPrecertificate(t *testing.T) {
	cert, err := fixchain.CertificateFromPEM(testonly.PrecertPEMValid)
	_, ok := err.(x509.NonFatalErrors)

	if err != nil && !ok {
		t.Fatalf("failed to set up test precert: %v", err)
	}

	km := setupMockKeyManager([]byte{0x77, 0xf3, 0x5c, 0xc6, 0xad, 0x85, 0xfd, 0xe0, 0x38, 0xfd, 0x36, 0x34, 0x5c, 0x1e, 0x45, 0x58, 0x60, 0x95, 0xb1, 0x7c, 0x28, 0xaa, 0xa5, 0xa5, 0x84, 0x96, 0x37, 0x4b, 0xf8, 0xbb, 0xd9, 0x8})

	leaf, got, err := signV1SCTForPrecertificate(km, cert, fixedTime)

	if err != nil {
		t.Fatalf("create sct for precert failed", err)
	}

	logID, err := base64.StdEncoding.DecodeString(ctMockLogID)

	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{SCTVersion: 0,
		LogID:      ct.SHA256Hash(idArray),
		Timestamp:  1504786523000000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{HashAlgorithm: ct.SHA256,
			SignatureAlgorithm: ct.RSA,
			Signature:          []byte("signed")}}

	assert.Equal(t, expected, got, "mismatched SCT (precert")

	// Additional checks that the MerkleTreeLeaf we built is correct
	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)

	assert.Equal(t, ct.V1, leaf.Version, "expected a v1 leaf")
	assert.Equal(t, ct.TimestampedEntryLeafType, leaf.LeafType, "expected a timestamped entry type")
	assert.Equal(t, ct.PrecertLogEntryType, leaf.TimestampedEntry.EntryType, "expected precert entry type")
	assert.Equal(t, got.Timestamp, leaf.TimestampedEntry.Timestamp, "entry / sct timestamp mismatch")
	assert.Equal(t, keyHash, leaf.TimestampedEntry.PrecertEntry.IssuerKeyHash, "issuer key hash mismatch")
	assert.Equal(t, cert.RawTBSCertificate, leaf.TimestampedEntry.PrecertEntry.TBSCertificate, "tbs cert mismatch")
}

func TestSerializeMerkleTreeLeafBadVersion(t *testing.T) {
	var leaf ct.MerkleTreeLeaf

	leaf.Version = 199 // Out of any expected valid range

	var b bytes.Buffer
	err := writeMerkleTreeLeaf(&b, leaf)
	assert.Error(t, err, "incorrectly serialized leaf with bad version")
	assert.Zero(t, len(b.Bytes()), "wrote data when serializing invalid object")
}

func TestSerializeMerkleTreeLeafBadType(t *testing.T) {
	var leaf ct.MerkleTreeLeaf

	leaf.Version = ct.V1
	leaf.LeafType = 212

	var b bytes.Buffer
	err := writeMerkleTreeLeaf(&b, leaf)
	assert.Error(t, err, "incorrectly serialized leaf with bad leaf type")
	assert.Zero(t, len(b.Bytes()), "wrote data when serializing invalid object")
}

func TestSerializeMerkleTreeLeafCert(t *testing.T) {
	ts := ct.TimestampedEntry{
		Timestamp:  12345,
		EntryType:  ct.X509LogEntryType,
		X509Entry:  ct.ASN1Cert([]byte{0x10, 0x11, 0x12, 0x13, 0x20, 0x21, 0x22, 0x23}),
		Extensions: ct.CTExtensions{}}
	leaf := ct.MerkleTreeLeaf{LeafType: ct.TimestampedEntryLeafType, Version: ct.V1, TimestampedEntry: ts}

	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	if err := writeMerkleTreeLeaf(w, leaf); err != nil {
		t.Fatalf("failed to write leaf: %v", err)
	}

	w.Flush()
	r := bufio.NewReader(&buff)

	leaf2, err := ct.ReadMerkleTreeLeaf(r)

	if err != nil {
		t.Fatalf("failed to read leaf: %v", err)
	}

	assert.Equal(t, leaf, *leaf2, "leaf mismatch after serialization roundtrip (cert)")
}

func TestSerializeMerkleTreePrecert(t *testing.T) {
	ts := ct.TimestampedEntry{Timestamp: 12345,
		EntryType: ct.PrecertLogEntryType,
		PrecertEntry: ct.PreCert{
			IssuerKeyHash:  [sha256.Size]byte{0x55, 0x56, 0x57, 0x58, 0x59},
			TBSCertificate: ct.ASN1Cert([]byte{0x10, 0x11, 0x12, 0x13, 0x20, 0x21, 0x22, 0x23})},
		Extensions: ct.CTExtensions{}}
	leaf := ct.MerkleTreeLeaf{LeafType: ct.TimestampedEntryLeafType, Version: ct.V1, TimestampedEntry: ts}

	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	if err := writeMerkleTreeLeaf(w, leaf); err != nil {
		t.Fatalf("failed to write leaf: %v", err)
	}

	w.Flush()
	r := bufio.NewReader(&buff)

	leaf2, err := ct.ReadMerkleTreeLeaf(r)

	if err != nil {
		t.Fatalf("failed to read leaf: %v", err)
	}

	assert.Equal(t, leaf, *leaf2, "leaf mismatch after serialization roundtrip (precert)")
}

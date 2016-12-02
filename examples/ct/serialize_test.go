package ct

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/fixchain"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian/examples/ct/testonly"
)

func TestSignV1SCTForCertificate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cert, err := fixchain.CertificateFromPEM(testonly.LeafSignedByFakeIntermediateCertPEM)
	if err != nil {
		t.Fatalf("failed to set up test cert: %v", err)
	}

	toSign, _ := hex.DecodeString("7052085a63895983fc768ebe0858891bcd4326e797ef3b7ed5996e7655afd7ab")
	km := setupMockKeyManager(mockCtrl, toSign)

	leaf, got, err := signV1SCTForCertificate(km, cert, nil, fixedTime)
	if err != nil {
		t.Fatalf("create sct for cert failed: %v", err)
	}

	logID, err := hex.DecodeString(ctMockLogID)
	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{
		SCTVersion: 0,
		LogID:      ct.LogID{KeyID: ct.SHA256Hash(idArray)},
		Timestamp:  1504786523000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			Algorithm: tls.SignatureAndHashAlgorithm{
				Hash:      tls.SHA256,
				Signature: tls.ECDSA},
			Signature: []byte("signed"),
		},
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Mismatched SCT (cert), got %v, expected %v", got, expected)
	}

	// Additional checks that the MerkleTreeLeaf we built is correct
	if got, want := leaf.Version, ct.V1; got != want {
		t.Fatalf("Got a %v leaf, expected a %v leaf", got, want)
	}
	if got, want := leaf.LeafType, ct.TimestampedEntryLeafType; got != want {
		t.Fatalf("Got leaf type %v, expected %v", got, want)
	}
	if got, want := leaf.TimestampedEntry.EntryType, ct.X509LogEntryType; got != want {
		t.Fatalf("Got entry type %v, expected %v", got, want)
	}
	if got, want := leaf.TimestampedEntry.Timestamp, got.Timestamp; got != want {
		t.Fatalf("Entry / sct timestamp mismatch; got %v, expected %v", got, want)
	}
	if got, want := leaf.TimestampedEntry.X509Entry.Data, cert.Raw; !reflect.DeepEqual(got, want) {
		t.Fatalf("Cert bytes mismatch, got %x, expected %x", got, want)
	}
}

func TestSignV1SCTForPrecertificate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cert, err := fixchain.CertificateFromPEM(testonly.PrecertPEMValid)
	_, ok := err.(x509.NonFatalErrors)

	if err != nil && !ok {
		t.Fatalf("failed to set up test precert: %v", err)
	}

	toSign, _ := hex.DecodeString("af6a0abbf6a67d14f17ba3c0f6271956ca4b19f4d75d79f24c787a80701d6aa8")
	km := setupMockKeyManager(mockCtrl, toSign)

	// Use the same cert as the issuer for convenience.
	leaf, got, err := signV1SCTForPrecertificate(km, cert, cert, fixedTime)
	if err != nil {
		t.Fatalf("create sct for precert failed: %v", err)
	}

	logID, err := hex.DecodeString(ctMockLogID)
	if err != nil {
		t.Fatalf("failed to decode test log id: %s", ctMockLogID)
	}

	var idArray [sha256.Size]byte
	copy(idArray[:], logID)

	expected := ct.SignedCertificateTimestamp{SCTVersion: 0,
		LogID:      ct.LogID{KeyID: ct.SHA256Hash(idArray)},
		Timestamp:  1504786523000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			Algorithm: tls.SignatureAndHashAlgorithm{
				Hash:      tls.SHA256,
				Signature: tls.ECDSA},
			Signature: []byte("signed")}}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Mismatched SCT (precert), got %v, expected %v", got, expected)
	}

	// Additional checks that the MerkleTreeLeaf we built is correct
	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)

	// Additional checks that the MerkleTreeLeaf we built is correct
	if got, want := leaf.Version, ct.V1; got != want {
		t.Fatalf("Got a %v leaf, expected a %v leaf", got, want)
	}
	if got, want := leaf.LeafType, ct.TimestampedEntryLeafType; got != want {
		t.Fatalf("Got leaf type %v, expected %v", got, want)
	}
	if got, want := leaf.TimestampedEntry.EntryType, ct.PrecertLogEntryType; got != want {
		t.Fatalf("Got entry type %v, expected %v", got, want)
	}
	if got, want := got.Timestamp, leaf.TimestampedEntry.Timestamp; got != want {
		t.Fatalf("Entry / sct timestamp mismatch; got %v, expected %v", got, want)
	}
	if got, want := keyHash[:], leaf.TimestampedEntry.PrecertEntry.IssuerKeyHash[:]; !bytes.Equal(got, want) {
		t.Fatalf("Issuer key hash bytes mismatch, got %v, expected %v", got, want)
	}
	if got, want := leaf.TimestampedEntry.PrecertEntry.TBSCertificate, cert.RawTBSCertificate; !bytes.Equal(got, want) {
		t.Fatalf("TBS cert mismatch, got %v, expected %v", got, want)
	}
}

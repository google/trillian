// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ct

import (
	"bytes"
	"crypto/sha256"
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

	signer, err := setupSigner(fakeSignature)
	if err != nil {
		t.Fatalf("could not create signer: %v", err)
	}

	leaf, got, err := signV1SCTForCertificate(signer, cert, nil, fixedTime)
	if err != nil {
		t.Fatalf("create sct for cert failed: %v", err)
	}

	expected := ct.SignedCertificateTimestamp{
		SCTVersion: 0,
		LogID:      ct.LogID{KeyID: demoLogID},
		Timestamp:  1504786523000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			Algorithm: tls.SignatureAndHashAlgorithm{
				Hash:      tls.SHA256,
				Signature: tls.ECDSA},
			Signature: fakeSignature,
		},
	}

	if !reflect.DeepEqual(*got, expected) {
		t.Fatalf("Mismatched SCT (cert), got \n%v, expected \n%v", got, expected)
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

	signer, err := setupSigner(fakeSignature)
	if err != nil {
		t.Fatalf("could not create signer: %v", err)
	}

	// Use the same cert as the issuer for convenience.
	leaf, got, err := signV1SCTForPrecertificate(signer, cert, cert, fixedTime)
	if err != nil {
		t.Fatalf("create sct for precert failed: %v", err)
	}

	expected := ct.SignedCertificateTimestamp{
		SCTVersion: 0,
		LogID:      ct.LogID{KeyID: demoLogID},
		Timestamp:  1504786523000,
		Extensions: ct.CTExtensions{},
		Signature: ct.DigitallySigned{
			Algorithm: tls.SignatureAndHashAlgorithm{
				Hash:      tls.SHA256,
				Signature: tls.ECDSA},
			Signature: fakeSignature}}

	if !reflect.DeepEqual(*got, expected) {
		t.Fatalf("Mismatched SCT (precert), got \n%v, expected \n%v", got, expected)
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
	defangedTBS, _ := x509.RemoveCTPoison(cert.RawTBSCertificate)
	if got, want := leaf.TimestampedEntry.PrecertEntry.TBSCertificate, defangedTBS; !bytes.Equal(got, want) {
		t.Fatalf("TBS cert mismatch, got %v, expected %v", got, want)
	}
}

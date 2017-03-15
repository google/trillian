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
	"crypto/sha256"
	"errors"
	"fmt"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
)

// signV1TreeHead signs a tree head for CT. The input STH should have been built from a
// backend response and already checked for validity.
func signV1TreeHead(signer *crypto.Signer, sth *ct.SignedTreeHead) error {
	sthBytes, err := ct.SerializeSTHSignatureInput(*sth)
	if err != nil {
		return err
	}

	signature, err := signer.Sign(sthBytes)
	if err != nil {
		return err
	}

	sth.TreeHeadSignature = ct.DigitallySigned{
		Algorithm: tls.SignatureAndHashAlgorithm{
			Hash: tls.SHA256,
			// This relies on the protobuf enum values matching the TLS-defined values.
			Signature: tls.SignatureAlgorithm(keys.SignatureAlgorithm(signer.Public())),
		},
		Signature: signature.Signature,
	}
	return nil
}

func buildV1MerkleTreeLeafForCert(cert, issuer *x509.Certificate, timeMillis uint64) (*ct.MerkleTreeLeaf, error) {
	leaf := ct.MerkleTreeLeaf{
		Version:  ct.V1,
		LeafType: ct.TimestampedEntryLeafType,
		TimestampedEntry: &ct.TimestampedEntry{
			Timestamp: timeMillis,
			EntryType: ct.X509LogEntryType,
			X509Entry: &ct.ASN1Cert{Data: cert.Raw},
		},
	}
	return &leaf, nil
}

func buildV1MerkleTreeLeafForPrecert(cert, issuer *x509.Certificate, timeMillis uint64) (*ct.MerkleTreeLeaf, error) {
	if issuer == nil {
		// Need issuer for the IssuerKeyHash
		return nil, errors.New("no issuer available for pre-certificate")
	}

	// For precerts we need to extract the relevant data from the Certificate container,
	// specifically the DER-encoded TBSCertificate, but with the CT poison extension removed.
	// (This is only possible using the CT specific modified version of the x509 library.)
	keyHash := sha256.Sum256(issuer.RawSubjectPublicKeyInfo)
	defangedTBS, err := x509.RemoveCTPoison(cert.RawTBSCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to remove poison extension: %v", err)
	}
	precert := ct.PreCert{
		IssuerKeyHash:  keyHash,
		TBSCertificate: defangedTBS,
	}

	timestampedEntry := ct.TimestampedEntry{
		Timestamp:    timeMillis,
		EntryType:    ct.PrecertLogEntryType,
		PrecertEntry: &precert,
	}
	leaf := ct.MerkleTreeLeaf{
		Version:          ct.V1,
		LeafType:         ct.TimestampedEntryLeafType,
		TimestampedEntry: &timestampedEntry,
	}
	return &leaf, nil
}

func buildV1SCT(signer *crypto.Signer, leaf *ct.MerkleTreeLeaf) (*ct.SignedCertificateTimestamp, error) {
	// Serialize SCT signature input to get the bytes that need to be signed
	sctInput := ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		Timestamp:  leaf.TimestampedEntry.Timestamp,
		Extensions: ct.CTExtensions{},
	}
	data, err := ct.SerializeSCTSignatureInput(sctInput, ct.LogEntry{Leaf: *leaf})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize SCT data: %v", err)
	}

	signature, err := signer.Sign(data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign SCT data: %v", err)
	}

	digitallySigned := ct.DigitallySigned{
		Algorithm: tls.SignatureAndHashAlgorithm{
			Hash: tls.SHA256,
			// This relies on the protobuf enum values matching the TLS-defined values.
			Signature: tls.SignatureAlgorithm(keys.SignatureAlgorithm(signer.Public())),
		},
		Signature: signature.Signature,
	}

	logID, err := GetCTLogID(signer.Public())
	if err != nil {
		return nil, fmt.Errorf("failed to get logID for signing: %v", err)
	}

	return &ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		LogID:      ct.LogID{KeyID: logID},
		Timestamp:  sctInput.Timestamp,
		Extensions: ct.CTExtensions{},
		Signature:  digitallySigned,
	}, nil
}

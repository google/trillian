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
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/trillian/crypto"
	spb "github.com/google/trillian/crypto/sigpb"
)

var (
	fixedTime       = time.Date(2017, 9, 7, 12, 15, 23, 0, time.UTC)
	ctTesttubeLogID = [32]byte{176, 204, 131, 229, 165, 249, 125, 107, 175, 124, 9, 204,
		40, 73, 4, 135, 42, 199, 232, 139, 19, 44, 99, 80, 183, 198, 253, 38, 225, 108, 108, 119}
	// Public key for Google Testtube log, taken from CT Github repository
	ctTesttubePublicKey = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEw8i8S7qiGEs9NXv0ZJFh6uuOmR2Q
7dPprzk9XNNGkUXjzqx2SDvRfiwKYwBljfWujozHESVPQyydGaHhkaSz/g==
-----END PUBLIC KEY-----`
)

// This test uses the testtube key rather than our test key so we can verify the
// result easily
func TestGetCTLogID(t *testing.T) {
	pk, err := crypto.PublicKeyFromPEM(ctTesttubePublicKey)
	if err != nil {
		t.Fatalf("unexpected error loading public key: %v", err)
	}

	got, err := GetCTLogID(pk)
	if err != nil {
		t.Fatalf("error getting logid: %v", err)
	}

	if want := ctTesttubeLogID; got != want {
		t.Errorf("logID: \n%v want \n%v", got, want)
	}
}

func TestSerializeLogEntry(t *testing.T) {
	ts := ct.TimestampedEntry{
		Timestamp:  12345,
		EntryType:  ct.X509LogEntryType,
		X509Entry:  &ct.ASN1Cert{Data: []byte{0x10, 0x11, 0x12, 0x13, 0x20, 0x21, 0x22, 0x23}},
		Extensions: ct.CTExtensions{}}
	leaf := ct.MerkleTreeLeaf{LeafType: ct.TimestampedEntryLeafType, Version: ct.V1, TimestampedEntry: &ts}

	for chainLength := 1; chainLength < 10; chainLength++ {
		chain := createCertChain(chainLength)

		logEntry := LogEntry{Leaf: leaf, Chain: chain}
		entryData, err := tls.Marshal(logEntry)
		if err != nil {
			t.Fatalf("failed to serialize log entry: %v", err)
		}

		var logEntry2 LogEntry
		rest, err := tls.Unmarshal(entryData, &logEntry2)
		if err != nil {
			t.Fatalf("failed to deserialize log entry: %v", err)
		} else if len(rest) > 0 {
			t.Error("trailing data after serialized log entry")
		}

		if !reflect.DeepEqual(logEntry, logEntry2) {
			t.Fatalf("log entry mismatch after serialization roundtrip, %v != %v", logEntry, logEntry2)
		}
	}
}

// Creates a mock key manager for use in interaction tests
func setupMockPrivateKeyManager(ctrl *gomock.Controller, toSign []byte) (*crypto.MockPrivateKeyManager, error) {
	pubkey, err := crypto.PublicKeyFromPEM(ctTesttubePublicKey)
	if err != nil {
		return nil, err
	}
	mockKeyManager := setupMockPrivateKeyManagerForSth(ctrl, toSign)
	mockKeyManager.EXPECT().Public().AnyTimes().Return(pubkey)
	mockKeyManager.EXPECT().SignatureAlgorithm().AnyTimes().Return(spb.DigitallySigned_ECDSA)
	return mockKeyManager, nil
}

// As above but we don't expect the call for a public key as we don't need it for an STH
func setupMockPrivateKeyManagerForSth(ctrl *gomock.Controller, toSign []byte) *crypto.MockPrivateKeyManager {
	mockKeyManager := crypto.NewMockPrivateKeyManager(ctrl)
	mockKeyManager.EXPECT().Sign(gomock.Any(), toSign, gomock.Any()).AnyTimes().Return([]byte("signed"), nil)
	return mockKeyManager
}

// Creates a dummy cert chain
func createCertChain(numCerts int) []ct.ASN1Cert {
	chain := make([]ct.ASN1Cert, 0, numCerts)

	for c := 0; c < numCerts; c++ {
		certBytes := make([]byte, c+2)

		for i := 0; i < c+2; i++ {
			certBytes[i] = byte(c)
		}

		chain = append(chain, ct.ASN1Cert{Data: certBytes})
	}

	return chain
}

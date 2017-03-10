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

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/testonly"
)

var (
	fixedTime     = time.Date(2017, 9, 7, 12, 15, 23, 0, time.UTC)
	demoLogID     = [32]byte{19, 56, 222, 93, 229, 36, 102, 128, 227, 214, 3, 121, 93, 175, 126, 236, 97, 217, 34, 32, 40, 233, 98, 27, 46, 179, 164, 251, 84, 10, 60, 57}
	fakeSignature = []byte("signed")
)

func TestGetCTLogID(t *testing.T) {
	pk, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		t.Fatalf("unexpected error loading public key: %v", err)
	}

	got, err := GetCTLogID(pk)
	if err != nil {
		t.Fatalf("error getting logid: %v", err)
	}

	if want := demoLogID; got != want {
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

// Creates a fake signer for use in interaction tests.
// It will always return fakeSig when asked to sign something.
func setupSigner(fakeSig []byte) (*crypto.Signer, error) {
	key, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	return crypto.NewSigner(testonly.NewSignerWithFixedSig(key, fakeSig)), nil
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

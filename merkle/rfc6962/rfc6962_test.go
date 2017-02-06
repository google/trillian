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

package rfc6962

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func MustDecodeBase64(b64 string) []byte {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

func MustHexDecode(b string) []byte {
	r, err := hex.DecodeString(b)
	if err != nil {
		panic(err)
	}
	return r
}

func TestEmptyRoot(t *testing.T) {
	h := Hasher{crypto.SHA256}
	// This root was calculated with the C++/Python sparse Merkle tree code in the
	// github.com/google/certificate-transparency repo.
	emptyRoot := MustDecodeBase64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")
	if got, want := h.NullHash(-1), emptyRoot; !bytes.Equal(got, want) {
		t.Errorf("Expected empty root. Got: \n%x\nWant:\n%x", got, want)
	}
}

func TestNullHashes(t *testing.T) {
	th := Hasher{crypto.SHA256}
	// Generate a test vector.
	numEntries := th.Size() * 8
	tests := make([][]byte, numEntries, numEntries)
	tests[numEntries-1] = th.HashLeaf([]byte{})
	for i := numEntries - 2; i >= 0; i-- {
		tests[i] = th.HashChildren(tests[i+1], tests[i+1])
	}

	if got, want := th.Size()*8, 256; got != want {
		t.Fatalf("th.Size()*8: %v, want %v", got, want)
	}

	/*
		// HashEmpty is not defined as a leaf node.
		if got, want := tests[255], th.HashEmpty(); !bytes.Equal(got, want) {
			t.Fatalf("tests[255]: \n%x, want:\n%x", got, want)
		}
		if got, want := th.NullHash(255), th.HashEmpty(); !bytes.Equal(got, want) {
			t.Fatalf("NullHash(255): \n%x, want:\n%x", got, want)
		}
	*/

	for i, test := range tests {
		if got, want := th.NullHash(i), test; !bytes.Equal(got, want) {
			t.Errorf("NullHash(%v): \n%x, want:\n%x", i, got, want)
		}
	}
}

func TestRfc6962Hasher(t *testing.T) {
	hasher := Hasher{crypto.SHA256}
	for _, test := range []struct {
		Description string
		want, got   []byte
	}{
		{
			"RFC962 Empty",
			// echo -n | sha256sum
			MustHexDecode("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			hasher.HashEmpty(),
		},
		{
			"RFC6962 Leaf",
			// echo -n 004C313233343536 | xxd -r -p | sha256sum
			MustHexDecode("395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56"),
			hasher.HashLeaf([]byte("L123456")),
		},
		{
			"RFC6962 Node",
			// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
			MustHexDecode("aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb"),
			hasher.HashChildren([]byte("N123"), []byte("N456")),
		},
	} {
		if !bytes.Equal(test.want, test.got) {
			t.Errorf("%v:=\n%x, want \n%x", test.Description, test.got, test.want)
		}
	}
}

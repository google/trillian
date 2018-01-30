// Copyright 2017 Google Inc. All Rights Reserved.
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

package crypto

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
)

var dh = testonly.MustHexDecode

// It's important that signatures don't change.
func TestHashLogRootKnownValue(t *testing.T) {
	expectedSig := dh("23536f6d65206279746573207468617420776f6e2774206368616e67652e2e2e2e2e2e2e000000000d843fd70000000009f93e410000000000000000")
	root := &trillian.SignedLogRoot{
		TimestampNanos: 226770903,
		RootHash:       []byte("Some bytes that won't change......."),
		TreeSize:       167329345,
	}
	hash, err := CanonicalLogRoot(root, LogRootV0)
	if err != nil {
		t.Fatalf("CanonicalLogRoot(): %v", err)
	}
	if got, want := hash, expectedSig; !bytes.Equal(got, want) {
		t.Fatalf("TestHashLogRootKnownValue: got:%x, want:%x", got, want)
	}
}

func TestHashLogRoot(t *testing.T) {
	unique := make(map[[32]byte]bool)
	fakeRootHash := []byte("Islington is a great place to live")
	for _, test := range []struct {
		root *trillian.SignedLogRoot
	}{
		{
			root: &trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       fakeRootHash,
				TreeSize:       2,
			},
		},
		{
			root: &trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       fakeRootHash,
				TreeSize:       2,
				LogId:          4,
			},
		},
		{
			root: &trillian.SignedLogRoot{
				TimestampNanos: 2267708,
				RootHash:       fakeRootHash,
				TreeSize:       3,
			},
		},
		{
			root: &trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Oslington is a great place to live"),
				TreeSize:       2,
			},
		},
		{
			root: &trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       fakeRootHash,
				TreeSize:       3,
			},
		},
	} {
		canonical, err := CanonicalLogRoot(test.root, LogRootV0)
		if err != nil {
			t.Fatalf("CanonicalLogRoot(): %v", err)
		}
		t.Logf("CanonicalRoot: %x", canonical)

		h := sha256.Sum256(canonical)
		if _, ok := unique[h]; ok {
			t.Errorf("Found duplicate canonical form for input %v", test.root)
		}
		unique[h] = true
	}

}

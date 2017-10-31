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
	"encoding/hex"
	"testing"

	"github.com/google/trillian"
)

// It's important that signatures don't change.
var expectedSigHex = "5e6baba8dc3465de9c01d669059dda590b7ce123d6ccd436bcd898f1c79ff6d9"

func TestHashLogRootKnownValue(t *testing.T) {
	root := trillian.SignedLogRoot{
		TimestampNanos: 226770903,
		RootHash:       []byte("Some bytes that won't change"),
		TreeSize:       167329345,
	}
	if got, want := hex.EncodeToString(HashLogRoot(root)), expectedSigHex; got != want {
		t.Fatalf("TestHashLogRootKnownValue: got:%v, want:%v", got, want)
	}
}

func TestHashLogRoot(t *testing.T) {
	unique := make(map[[20]byte]bool)
	for _, test := range []struct {
		root trillian.SignedLogRoot
	}{
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Islington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267708,
				RootHash:       []byte("Islington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Oslington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Islington"),
				TreeSize:       3,
			},
		},
	} {
		hash := HashLogRoot(test.root)
		var h [20]byte
		copy(h[:], hash)
		if _, ok := unique[h]; ok {
			t.Errorf("Found duplicate hash from input %v", test.root)
		}
		unique[h] = true
	}

}

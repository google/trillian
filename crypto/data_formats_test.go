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
	"testing"

	"github.com/google/trillian"
)

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

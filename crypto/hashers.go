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

package crypto

import (
	"crypto"
	_ "crypto/sha256" // Register the SHA256 algorithm
	"fmt"

	"github.com/google/trillian"
)

var hashLookup = map[trillian.HashAlgorithm]crypto.Hash{
	trillian.HashAlgorithm_SHA256: crypto.SHA256,
}

var cipherLookup = map[crypto.Hash]trillian.HashAlgorithm{
	crypto.SHA256: trillian.HashAlgorithm_SHA256,
}

// LookupHash returns the go crypto hash algorithm associated with the trillian hash enum.
func LookupHash(hash trillian.HashAlgorithm) (crypto.Hash, error) {
	hasher, ok := hashLookup[hash]
	if !ok {
		return 0, fmt.Errorf("Unsupported hash algorithm %v", hash)
	}
	return hasher, nil
}

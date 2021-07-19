// Copyright 2017 Google LLC. All Rights Reserved.
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

package testonly

// This file implements the hashing functions that are part of a Trillian
// personality.

import "crypto/sha256"

// HashKey converts a map key into a map index using SHA256.
// This preserves tests that precomputed indexes based on SHA256.
func HashKey(key string) []byte {
	h := sha256.New()
	h.Write([]byte(key))
	return h.Sum(nil)
}

// TransparentHash returns a key that can be visually inspected.
// This supports testing where it was nice to see what the key was.
func TransparentHash(key string) []byte {
	const prefixLen = 8
	if prefixLen+len(key) > sha256.Size {
		panic("key too long")
	}
	b := make([]byte, sha256.Size)
	// Put some hashed bytes before the key so that key values are
	// a bit more widely distributed around the Merkle tree.
	h := HashKey(key)
	copy(b[0:prefixLen], h[0:prefixLen])
	copy(b[prefixLen:], key)
	return b
}

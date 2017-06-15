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

// Package objhasher provides generic object hashing functionality.
package objhasher

import (
	"crypto"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/maphasher"
)

// ObjectHasher uses ObjectHash to compute leaf hashes.
var ObjectHasher merkle.TreeHasher = &objhasher{
	// Use SHA256 to match ObjectHash.
	TreeHasher: maphasher.New(crypto.SHA256),
}

// ObjectHash does not use `1` as any of its type prefixes,
// preserving domain separation.
const nodeHashPrefix = 1

type objhasher struct {
	merkle.TreeHasher
}

// HashLeaf returns the object hash of leaf, which must be a JSON object.
func (o *objhasher) HashLeaf(leaf []byte) []byte {
	hash := objecthash.CommonJSONHash(string(leaf))
	return hash[:]
}

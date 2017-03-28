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

package objhasher

import (
	"crypto/sha256" // Use SHA256 to match ObjectHash.

	"github.com/benlaurie/objecthash/go/objecthash"
)

// ObjectHasher uses ObjectHash to compute leaf hashes.
var ObjectHasher = &objhasher{}

// ObjectHash does not use `1` as any of its type prefixes,
// preserving domain separation.
const nodeHashPrefix = 1

type objhasher struct{}

// HashEmpty returns the hash of an empty element for the tree
func (o *objhasher) HashEmpty() []byte {
	return sha256.New().Sum(nil)
}

// HashLeaf returns the object hash of leaf, which must be a JSON object.
func (o *objhasher) HashLeaf(leaf []byte) []byte {
	hash := objecthash.CommonJSONHash(string(leaf))
	return hash[:]
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (o *objhasher) HashChildren(l, r []byte) []byte {
	h := sha256.New()
	h.Write([]byte{nodeHashPrefix})
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

// Size returns the number of bytes in the hash output.
func (o *objhasher) Size() int {
	return sha256.Size
}

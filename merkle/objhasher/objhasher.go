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
	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian/merkle/hashers"
)

type objmaphasher struct {
	hashers.MapHasher
}

type objloghasher struct {
	hashers.LogHasher
}

// NewMapHasher returns a new ObjectHasher based on the passed in MapHasher
func NewMapHasher(baseHasher hashers.MapHasher) hashers.MapHasher {
	return &objmaphasher{
		MapHasher: baseHasher,
	}
}

// NewLogHasher returns a new ObjectHasher based on the passed in MapHasher
func NewLogHasher(baseHasher hashers.LogHasher) hashers.LogHasher {
	return &objloghasher{
		LogHasher: baseHasher,
	}
}

// HashLeaf returns the object hash of leaf, which must be a JSON object.
func (o *objloghasher) HashLeaf(leaf []byte) []byte {
	hash := objecthash.CommonJSONHash(string(leaf))
	return hash[:]
}

// HashLeaf returns the object hash of leaf, which must be a JSON object.
func (o *objmaphasher) HashLeaf(leaf []byte) []byte {
	hash := objecthash.CommonJSONHash(string(leaf))
	return hash[:]
}

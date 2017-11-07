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
	"fmt"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/rfc6962"
)

func init() {
	hashers.RegisterLogHasher(trillian.HashStrategy_OBJECT_RFC6962_SHA256, NewLogHasher(rfc6962.New(crypto.SHA256)))
}

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
func (o *objloghasher) HashLeaf(leaf []byte) ([]byte, error) {
	hash, err := objecthash.CommonJSONHash(string(leaf))
	if err != nil {
		return nil, fmt.Errorf("CommonJSONHash(%s): %v", leaf, err)
	}
	return hash[:], err
}

// HashLeaf returns the object hash of leaf, which must be a JSON object.
func (o *objmaphasher) HashLeaf(treeID int64, index []byte, leaf []byte) ([]byte, error) {
	hash, err := objecthash.CommonJSONHash(string(leaf))
	if err != nil {
		return nil, fmt.Errorf("CommonJSONHash(%s): %v", leaf, err)
	}
	return hash[:], nil
}

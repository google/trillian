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

package merkle

import (
	"crypto"
	"fmt"

	"github.com/google/trillian/merkle/rfc6962"
)

const (
	// RFC6962SHA256Type is the string used to retrieve the RFC6962 hasher.
	RFC6962SHA256Type = "RFC6962-SHA256"
)

// TreeHasher is the interface that the previous tree hasher struct implemented.
type TreeHasher interface {
	HashEmpty() []byte
	HashLeaf(leaf []byte) []byte
	HashChildren(l, r []byte) []byte
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}

var hashTypes = map[string]TreeHasher{
	RFC6962SHA256Type: rfc6962.TreeHasher{Hash: crypto.SHA256},
}

// Factory supports fetching custom hashers based on tree types.
func Factory(hashType string) (TreeHasher, error) {
	h, ok := hashTypes[hashType]
	if !ok {
		return nil, fmt.Errorf("hash type %s not found", hashType)
	}
	return h, nil
}

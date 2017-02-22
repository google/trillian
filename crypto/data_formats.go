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
	"encoding/base64"
	"strconv"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
)

// This file contains struct specific mappings and data structures.
// TODO(gdbelvin): remove data-structure specific operations.

// Constants used as map keys when building input for ObjectHash. They must not be changed
// as this will change the output of hashRoot()
const (
	mapKeyRootHash       string = "RootHash"
	mapKeyTimestampNanos string = "TimestampNanos"
	mapKeyTreeSize       string = "TreeSize"
)

// HashLogRoot hashes SignedLogRoot objects in a custom way
// using the map of properties listed above.
func HashLogRoot(root trillian.SignedLogRoot) []byte {
	// Pull out the fields we want to hash.
	// Caution: use string format for int64 values as they can overflow when
	// JSON encoded otherwise (it uses floats). We want to be sure that people
	// using JSON to verify hashes can build the exact same input to ObjectHash.
	rootMap := map[string]string{
		mapKeyRootHash:       base64.StdEncoding.EncodeToString(root.RootHash),
		mapKeyTimestampNanos: strconv.FormatInt(root.TimestampNanos, 10),
		mapKeyTreeSize:       strconv.FormatInt(root.TreeSize, 10)}

	hash := objecthash.ObjectHash(rootMap)
	return hash[:]
}

// SignLogRoot returns a signature using objecthash on a fixed JSON format of the root.
func (s *Signer) SignLogRoot(root trillian.SignedLogRoot) (*sigpb.DigitallySigned, error) {
	return s.Sign(HashLogRoot(root))
}

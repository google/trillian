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
	"fmt"
	"strconv"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian"
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

// HashLogRoot hashes SignedLogRoot objects using ObjectHash with
// "RootHash", "TimestampNanos", and "TreeSize", used as keys in
// a map.
func HashLogRoot(root trillian.SignedLogRoot) ([]byte, error) {
	// Pull out the fields we want to hash.
	// Caution: use string format for int64 values as they can overflow when
	// JSON encoded otherwise (it uses floats). We want to be sure that people
	// using JSON to verify hashes can build the exact same input to ObjectHash.
	rootMap := map[string]interface{}{
		mapKeyRootHash:       base64.StdEncoding.EncodeToString(root.RootHash),
		mapKeyTimestampNanos: strconv.FormatInt(root.TimestampNanos, 10),
		mapKeyTreeSize:       strconv.FormatInt(root.TreeSize, 10)}

	hash, err := objecthash.ObjectHash(rootMap)
	if err != nil {
		return nil, fmt.Errorf("ObjectHash(%#v): %v", rootMap, err)
	}
	return hash[:], nil
}

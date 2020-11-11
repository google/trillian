// Copyright 2020 Google LLC. All Rights Reserved.
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

// verify is a simple example that shows how a verifiable map can be used to
// demonstrate inclusion.
package main

import (
	"bytes"
	"crypto"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/experimental/batchmap/tilepb"
	"github.com/google/trillian/merkle/coniks"
)

const hash = crypto.SHA512_256

var (
	mapDir       = flag.String("map_dir", "", "Directory containing map tiles.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	valueSalt    = flag.String("value_salt", "v1", "Some string that will be smooshed in with the generated value before hashing. Allows generated values to be deterministic but variable.")
	key          = flag.Int64("start_key", 0, "Keys will be generated starting with this index.")
	prefixStrata = flag.Int("prefix_strata", 1, "The number of strata of 8-bit strata before the final strata.")
)

func main() {
	flag.Parse()

	mapDir := filepath.Clean(*mapDir)
	if mapDir == "" {
		glog.Fatal("No output provided")
	}

	// Determine the key/value we expect to find.
	// Note that the map tiles do not contain raw values, but commitments to the values.
	// If the map needs to return the values to clients then it is recommended that the
	// map operator uses a Content Addressable Store to store these values.
	h := hash.New()
	h.Write([]byte(fmt.Sprintf("%d", *key)))
	keyPath := h.Sum(nil)

	expectedString := fmt.Sprintf("[%s]%d", *valueSalt, *key)
	expectedValueHash := coniks.Default.HashLeaf(*treeID, keyPath, []byte(expectedString))

	// Read the tiles required for this check from disk.
	tiles := make([]*tilepb.Tile, *prefixStrata+1)
	for i := 0; i <= *prefixStrata; i++ {
		tilePath := keyPath[0:i]
		tileFile := fmt.Sprintf("%s/path_%x", mapDir, tilePath)
		in, err := ioutil.ReadFile(tileFile)
		if err != nil {
			glog.Fatal(err)
		}
		tile := &tilepb.Tile{}
		if err := proto.Unmarshal(in, tile); err != nil {
			glog.Fatalf("failed to parse tile in %s: %v", tileFile, err)
		}
		tiles[i] = tile
	}

	// Confirm the expected value in the leaf tile.
	leafTile := tiles[len(tiles)-1]
	expectedLeafPath := keyPath[len(leafTile.Path):]
	var leaf *tilepb.TileLeaf
	for _, l := range leafTile.Leaves {
		if bytes.Equal(l.Path, expectedLeafPath) {
			leaf = l
		}
	}
	if leaf == nil {
		glog.Fatalf("couldn't find expected key %x", keyPath)
	}
	if !bytes.Equal(leaf.Hash, expectedValueHash) {
		glog.Fatalf("value for key %x incorrect, got %x but expeted %x", keyPath, leaf.Hash, expectedValueHash)
	}
	glog.Infof("key %d found at path %x, with value '%s' (%x)", *key, keyPath, expectedString, expectedValueHash)

	// TODO(mhutchinson): Demonstrate that the root hash commits to this key/value.
}

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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle"
	coniks "github.com/google/trillian/merkle/coniks/hasher"
)

const hash = crypto.SHA512_256

var (
	mapDir       = flag.String("map_dir", "", "Directory containing map tiles.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	valueSalt    = flag.String("value_salt", "v1", "Some string that will be smooshed in with the generated value before hashing. Allows generated values to be deterministic but variable.")
	key          = flag.Int64("key", 0, "This is the seed for the key that will be looked up.")
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
	tiles, err := getTilesForKey(mapDir, keyPath)
	if err != nil {
		glog.Exitf("couldn't load tiles: %v", err)
	}

	// Perform the verification.
	// 1) Start at the leaf tile and check the key/value.
	// 2) Compute the merkle root of the leaf tile
	// 3) Check the computed root matches that reported in the tile
	// 4) Check this root value is the key/value of the tile above.
	// 5) Rinse and repeat until we reach the tree root.
	hs2 := merkle.NewHStar2(*treeID, coniks.Default)
	needPath, needValue := keyPath, expectedValueHash

	for i := *prefixStrata; i >= 0; i-- {
		tile := tiles[i]
		// Check the prefix of what we are looking for matches the tile's path.
		if got, want := tile.Path, needPath[:len(tile.Path)]; !bytes.Equal(got, want) {
			glog.Fatalf("wrong tile found at index %d: got %x, want %x", i, got, want)
		}
		// Leaf paths within a tile are within the scope of the tile, so we can
		// drop the prefix from the expected path now we have verified it.
		needLeafPath := needPath[len(tile.Path):]

		// Identify the leaf we need, and convert all leaves to the format needed for hashing.
		var leaf *batchmap.TileLeaf
		hs2Leaves := make([]*merkle.HStar2LeafHash, len(tile.Leaves))
		for j, l := range tile.Leaves {
			if bytes.Equal(l.Path, needLeafPath) {
				leaf = l
			}
			hs2Leaves[j] = toHStar2(tile.Path, l)
		}

		// Confirm we found the leaf we needed, and that it had the value we expected.
		if leaf == nil {
			glog.Fatalf("couldn't find expected leaf %x in tile %x", needLeafPath, tile.Path)
		}
		// TODO(pavelkalinnikov): Remove nolint after fixing
		// https://github.com/dominikh/go-tools/issues/921.
		if !bytes.Equal(leaf.Hash, needValue) { // nolint: staticcheck
			glog.Fatalf("wrong leaf value in tile %x, leaf %x: got %x, want %x", tile.Path, leaf.Path, leaf.Hash, needValue)
		}

		// Hash this tile given its leaf values, and confirm that the value we compute
		// matches the value reported in the tile.
		root, err := hs2.HStar2Nodes(tile.Path, 8*len(leaf.Path), hs2Leaves, nil, nil)
		if err != nil {
			glog.Fatalf("failed to hash tile %x: %v", tile.Path, err)
		}
		if !bytes.Equal(root, tile.RootHash) {
			glog.Fatalf("wrong root hash for tile %x: got %x, calculated %x", tile.Path, tile.RootHash, root)
		}

		// Make the next iteration of the loop check that the tile above this has the
		// root value of this tile stored as the value at the expected leaf index.
		needPath, needValue = tile.Path, root
	}

	// If we get here then we have proved that the value was correct and that the map
	// root commits to this value. Any other user with the same map root must see the
	// same value under the same key we have checked.
	glog.Infof("key %d found at path %x, with value '%s' (%x) committed to by map root %x", *key, keyPath, expectedString, expectedValueHash, needValue)
}

// getTilesForKey loads the tiles on the path from the root to the given leaf.
func getTilesForKey(mapDir string, key []byte) ([]*batchmap.Tile, error) {
	tiles := make([]*batchmap.Tile, *prefixStrata+1)
	for i := 0; i <= *prefixStrata; i++ {
		tilePath := key[0:i]
		tileFile := fmt.Sprintf("%s/path_%x", mapDir, tilePath)
		in, err := ioutil.ReadFile(tileFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %v", tileFile, err)
		}
		tile := &batchmap.Tile{}
		if err := json.Unmarshal(in, tile); err != nil {
			return nil, fmt.Errorf("failed to parse tile in %s: %v", tileFile, err)
		}
		tiles[i] = tile
	}
	return tiles, nil
}

// toHStar2 converts a TileLeaf into the equivalent structure for HStar2.
func toHStar2(prefix []byte, l *batchmap.TileLeaf) *merkle.HStar2LeafHash {
	// In hstar2 all paths need to be 256 bit (32 bytes)
	leafIndexBs := make([]byte, 32)
	copy(leafIndexBs, prefix)
	copy(leafIndexBs[len(prefix):], l.Path)
	return &merkle.HStar2LeafHash{
		Index:    new(big.Int).SetBytes(leafIndexBs),
		LeafHash: l.Hash,
	}
}

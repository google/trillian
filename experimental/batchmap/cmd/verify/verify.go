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
	"path/filepath"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/merkle/smt/node"
	"k8s.io/klog/v2"
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
	klog.InitFlags(nil)
	flag.Parse()

	mapDir := filepath.Clean(*mapDir)
	if mapDir == "" {
		klog.Fatal("No output provided")
	}

	// Determine the key/value we expect to find.
	// Note that the map tiles do not contain raw values, but commitments to the values.
	// If the map needs to return the values to clients then it is recommended that the
	// map operator uses a Content Addressable Store to store these values.
	h := hash.New()
	h.Write([]byte(fmt.Sprintf("%d", *key)))
	keyPath := h.Sum(nil)
	leafID := node.NewID(string(keyPath), uint(len(keyPath)*8))

	expectedString := fmt.Sprintf("[%s]%d", *valueSalt, *key)
	expectedValueHash := coniks.Default.HashLeaf(*treeID, leafID, []byte(expectedString))

	// Read the tiles required for this check from disk.
	tiles, err := getTilesForKey(mapDir, keyPath)
	if err != nil {
		klog.Exitf("couldn't load tiles: %v", err)
	}

	// Perform the verification.
	// 1) Start at the leaf tile and check the key/value.
	// 2) Compute the merkle root of the leaf tile
	// 3) Check the computed root matches that reported in the tile
	// 4) Check this root value is the key/value of the tile above.
	// 5) Rinse and repeat until we reach the tree root.
	et := emptyTree{treeID: *treeID}
	needPath, needValue := keyPath, expectedValueHash

	for i := *prefixStrata; i >= 0; i-- {
		tile := tiles[i]
		// Check the prefix of what we are looking for matches the tile's path.
		if got, want := tile.Path, needPath[:len(tile.Path)]; !bytes.Equal(got, want) {
			klog.Fatalf("wrong tile found at index %d: got %x, want %x", i, got, want)
		}
		// Leaf paths within a tile are within the scope of the tile, so we can
		// drop the prefix from the expected path now we have verified it.
		needLeafPath := needPath[len(tile.Path):]

		// Identify the leaf we need, and convert all leaves to the format needed for hashing.
		var leaf *batchmap.TileLeaf
		nodes := make([]smt.Node, len(tile.Leaves))
		for j, l := range tile.Leaves {
			if bytes.Equal(l.Path, needLeafPath) {
				leaf = l
			}
			nodes[j] = toNode(tile.Path, l)
		}

		// Confirm we found the leaf we needed, and that it had the value we expected.
		if leaf == nil { //nolint:staticcheck // Remove "related info" lint message for suppressed lint check below.
			klog.Fatalf("couldn't find expected leaf %x in tile %x", needLeafPath, tile.Path)
		}
		if !bytes.Equal(leaf.Hash, needValue) { //nolint:staticcheck // Suppress false +ve due to linter not understanding that klog.Fatal() above will exit if leaf == nil
			klog.Fatalf("wrong leaf value in tile %x, leaf %x: got %x, want %x", tile.Path, leaf.Path, leaf.Hash, needValue)
		}

		// Hash this tile given its leaf values, and confirm that the value we compute
		// matches the value reported in the tile.
		hs, err := smt.NewHStar3(nodes, coniks.Default.HashChildren,
			uint(len(tile.Path)+len(leaf.Path))*8, uint(len(tile.Path))*8)
		if err != nil {
			klog.Fatalf("failed to create HStar3 for tile %x: %v", tile.Path, err)
		}
		res, err := hs.Update(et)
		if err != nil {
			klog.Fatalf("failed to hash tile %x: %v", tile.Path, err)
		} else if got, want := len(res), 1; got != want {
			klog.Fatalf("wrong number of roots for tile %x: got %v, want %v", tile.Path, got, want)
		}
		if got, want := res[0].Hash, tile.RootHash; !bytes.Equal(got, want) {
			klog.Fatalf("wrong root hash for tile %x: got %x, calculated %x", tile.Path, want, got)
		}
		// Make the next iteration of the loop check that the tile above this has the
		// root value of this tile stored as the value at the expected leaf index.
		needPath, needValue = tile.Path, res[0].Hash
	}

	// If we get here then we have proved that the value was correct and that the map
	// root commits to this value. Any other user with the same map root must see the
	// same value under the same key we have checked.
	klog.Infof("key %d found at path %x, with value '%s' (%x) committed to by map root %x", *key, keyPath, expectedString, expectedValueHash, needValue)
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

// toNode converts a TileLeaf into the equivalent Node for HStar3.
func toNode(prefix []byte, l *batchmap.TileLeaf) smt.Node {
	path := make([]byte, 0, len(prefix)+len(l.Path))
	path = append(append(path, prefix...), l.Path...)
	return smt.Node{
		ID:   node.NewID(string(path), uint(len(path))*8),
		Hash: l.Hash,
	}
}

// emptyTree is a NodeAccessor for an empty tree with the given ID.
type emptyTree struct {
	treeID int64
}

func (e emptyTree) Get(id node.ID) ([]byte, error) {
	return coniks.Default.HashEmpty(e.treeID, id), nil
}

func (e emptyTree) Set(id node.ID, hash []byte) {}

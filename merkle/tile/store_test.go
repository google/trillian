// Copyright 2019 Google Inc. All Rights Reserved.
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

package tile

import (
	"bytes"
	"errors"
	"fmt"
	"math/bits"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
)

// testLeaf returns leaf value data for a test leaf of the given index.
// All Merkle trees in this file use this for their leaf data.
func testLeaf(idx uint64) []byte {
	return []byte(fmt.Sprintf("leaf-%d", idx))
}

// storeConstructor builds a new Store instance.
type storeConstructor func(tileHeight uint64, hashChildren func(l, r []byte) []byte) Store

func testInvalidStoreHeight(t *testing.T, constructor storeConstructor) {
	heights := []uint64{0, 1, 65}
	for _, height := range heights {
		t.Run(fmt.Sprintf("tileHeight-%d", height), func(t *testing.T) {
			defer func() {
				recover()
			}()
			// Invalid heights cause panic.
			constructor(height, h2)
			t.Fatalf("constructor(%d) succeeded, wanted panic", height)
		})
	}
}

func testStoreSmall(t *testing.T, constructor storeConstructor) {
	// This test uses a tile height of 4.
	ts := constructor(4, h2)
	if got, want := ts.TileHeight(), uint64(4); got != want {
		t.Fatalf("ts.TileHeight()=%d, want %d", got, want)
	}
	t00 := tile4(0, 0)
	t01 := tile4(0, 1)
	t00alt := tile4(0, 0)  // with different hashes
	t01some := tile4(0, 1) // with a subset of hashes
	t01some.Hashes = t01.Hashes[:2]

	tests := []struct {
		add     bool
		tile    *Tile
		wantErr string
	}{
		{tile: t00, wantErr: "not available"},
		{add: true, tile: t00},
		{tile: t00},
		{tile: t01, wantErr: "not available"},
		{add: true, tile: t00alt, wantErr: "extension tile has different hash"},
		{add: true, tile: t01some},
		{tile: t01some},
		{add: true, tile: t01}, // OK to extend
		{tile: t01},
		{add: true, tile: t01some, wantErr: "extension tile has fewer hashes"}, // not OK to contract
		{tile: tile2(1, 0), wantErr: "invalid tile height"},
		{add: true, tile: tile2(1, 0), wantErr: "invalid tile height"},
	}
	// Note: tests are cumulative.
	for _, test := range tests {
		if test.add {
			err := ts.Add(test.tile)
			if err != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("ts.Add(%v)=%v, want nil", test.tile, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("ts.Add(%v)=%v, want err containing %q", test.tile, err, test.wantErr)
				}
			} else if len(test.wantErr) > 0 {
				t.Errorf("ts.Add(%v)=nil, want err containing %q", test.tile, test.wantErr)
			}
		} else {
			got, err := ts.TileAt(test.tile.Coords)
			if err != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("ts.TileAt(%v)=nil,%v, want %v,nil", test.tile.Coords, err, test.tile)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("ts.TileAt(%v)=nil,%v, want nil,err containing %q", test.tile.Coords, err, test.wantErr)
				}
			} else if len(test.wantErr) > 0 {
				t.Errorf("ts.TileAt(%v)=%v,nil, want nil,err containing %q", test.tile.Coords, got, test.wantErr)
			} else if !reflect.DeepEqual(got, test.tile) {
				t.Errorf("ts.TileAt(%v)=%v,nil, want %v,nil", test.tile.Coords, got, test.tile)
			}
		}
	}
}

func TestRequiredTilesErrors(t *testing.T) {
	tests := []struct {
		err  RequiredTilesError
		want string
	}{
		{
			err:  RequiredTilesError{},
			want: "tiles required:",
		},
		{
			err: RequiredTilesError{
				Missing: []MissingTileError{{Coords: tc(2, 0, 0)}},
			},
			want: "tiles required: missing: tile2(0,0)",
		},
		{
			err: RequiredTilesError{
				Missing: []MissingTileError{{Coords: tc(2, 0, 0)}, {Coords: tc(2, 0, 1)}},
			},
			want: "tiles required: missing: tile2(0,0),tile2(0,1)",
		},
		{
			err: RequiredTilesError{
				Incomplete: []IncompleteTileError{
					{Coords: tc(2, 0, 0), Index: 3, Size: 1},
				},
			},
			want: "tiles required: incomplete: tile2(0,0)/1-need-/4",
		},
		{
			err: RequiredTilesError{
				Incomplete: []IncompleteTileError{
					{Coords: tc(2, 1, 0), Index: 3, Size: 1},
					{Coords: tc(2, 0, 5), Index: 3, Size: 2},
				},
			},
			want: "tiles required: incomplete: tile2(1,0)/1-need-/4,tile2(0,5)/2-need-/4",
		},
	}
	for _, test := range tests {
		got := test.err.Error()
		if got != test.want {
			t.Errorf("err.String()=%q, want %q", got, test.want)
		}
	}
}

func TestCombineErrors(t *testing.T) {
	normal1 := errors.New("normal1")
	normal2 := errors.New("normal2")
	missing1 := RequiredTilesError{Missing: []MissingTileError{{Coords: tc(2, 0, 0)}}}
	missing2 := RequiredTilesError{Missing: []MissingTileError{{Coords: tc(2, 0, 1)}}}
	missingBoth := RequiredTilesError{Missing: []MissingTileError{{Coords: tc(2, 0, 0)}, {Coords: tc(2, 0, 1)}}}
	tests := []struct {
		l, r, want error
	}{
		{l: nil, r: nil, want: nil},
		{l: normal1, r: nil, want: normal1},
		{l: missing1, r: nil, want: missing1},
		{l: nil, r: normal2, want: normal2},
		{l: normal1, r: normal2, want: normal1},
		{l: missing1, r: normal2, want: normal2},
		{l: nil, r: missing2, want: missing2},
		{l: normal1, r: missing2, want: normal1},
		{l: missing1, r: missing2, want: missingBoth},
	}
	for _, test := range tests {
		got := combineErrs(test.l, test.r)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("combineErrs(%v,%v)=%v want %v", test.l, test.r, got, test.want)
		}
	}
}

// treeInfo builds a tree of the given size (using testLeaf() for leaf values) and
// returns a map of coordinates to node hash value, and a text summary of the hash
// tree (with showHashCount bytes of each hash displayed).
func treeInfo(treeSize uint64, showHashCount int) (map[MerkleCoords][]byte, string) {
	hashWidth := showHashCount * 2 // two hex digits per byte of hash displayed
	hasher := rfc6962.DefaultHasher
	nodes := make([][]byte, treeSize)
	for leafIdx := uint64(0); leafIdx < treeSize; leafIdx++ {
		newLeaf := testLeaf(leafIdx)
		hash, _ := hasher.HashLeaf(newLeaf)
		nodes[leafIdx] = hash
	}

	hashes := make(map[MerkleCoords][]byte)
	treeHeight := bits.Len64(treeSize)
	gap := 1.0
	var results []string
	for level := 0; level < treeHeight; level++ {
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("L%d: ", level))
		for i, node := range nodes {
			prefix := strings.Repeat(" ", int(gap))
			if i == 0 {
				prefix = strings.Repeat(" ", int(gap/2.0))
			}
			buf.WriteString(fmt.Sprintf("%s%x", prefix, node[:showHashCount]))
			hashes[MerkleCoords{Level: uint64(level), Index: uint64(i)}] = node
		}

		results = append(results, buf.String())
		// 4 nodes at level n occupy 4*(hashWidth + gap_n)
		// 2 nodes at level n+1 occupy 2*hashWidth + 2*gap_n+1
		// => 2*gap_n+1 = 2*hashWidth + 4*gap_n
		// => gap_n+1 = hashWidth + 2*gap_n
		gap = float64(hashWidth) + 2.0*gap

		// Populate the (complete) hashes for the next level.
		nextLevelCount := len(nodes) / 2
		for i := 0; i < nextLevelCount; i++ {
			nodes[i] = hasher.HashChildren(nodes[2*i], nodes[2*i+1])
		}
		nodes = nodes[:nextLevelCount]
	}
	for i := len(results)/2 - 1; i >= 0; i-- {
		j := len(results) - 1 - i
		results[i], results[j] = results[j], results[i]
	}
	return hashes, strings.Join(results, "\n")
}

// buildTreeAndTiles populates a Store and also builds an in-memory Merkle
// tree of the given size, using testLeaf() for leaf values.
func buildTreeAndTiles(t *testing.T, ts Store, treeSize uint64) *merkle.InMemoryMerkleTree {
	t.Helper()

	// Build the tree by creating leaves, populating layer 0 tiles along the way.
	th := ts.TileHeight()
	tileWidth := uint64(1) << th
	hashes := make([][]byte, tileWidth)
	imt := merkle.NewInMemoryMerkleTree(rfc6962.DefaultHasher)
	curHashIdx := uint64(0)
	tileIndex := uint64(0)
	for leafIdx := uint64(0); leafIdx < treeSize; leafIdx++ {
		newLeaf := testLeaf(leafIdx)
		seq, entry, err := imt.AddLeaf(newLeaf)
		if err != nil {
			t.Fatalf("imt.AddLeaf()=%d,%x,%v; want _,_,nil", seq, entry.Hash(), err)
		}
		if uint64(seq-1) != leafIdx {
			t.Fatalf("imt.AddLeaf()=%d,%x,nil; want %d,_,nil", seq, entry.Hash(), leafIdx)
		}

		glog.V(2).Infof("store leaf 0.%d in tile%d(0,%d).Hashes[%d] = %x", leafIdx, th, tileIndex, curHashIdx, entry.Hash()[:8])
		hashes[curHashIdx] = entry.Hash()
		curHashIdx++
		if curHashIdx == tileWidth {
			// Have a complete level 0 tile, add it to the store.
			coords := Coords{TileHeight: th, Level: 0, Index: tileIndex}
			tile, err := NewTile(coords, hashes, h2)
			if err != nil {
				t.Fatalf("NewTile(%v)=nil,%v; want _,nil", coords, err)
			}
			ts.Add(tile)
			glog.V(2).Infof("add complete %v=tile%d(0,%d) to store", tile, th, tileIndex)
			curHashIdx = 0
			tileIndex++
			hashes = make([][]byte, tileWidth)
		}
	}
	// There may be a final incomplete tile.
	if curHashIdx != 0 {
		coords := Coords{TileHeight: th, Level: 0, Index: tileIndex}
		tile, err := NewTile(coords, hashes[:curHashIdx], h2)
		if err != nil {
			t.Fatalf("NewTile(%v)=nil,%v; want _,nil", coords, err)
		}
		glog.V(2).Infof("add incomplete complete %v=tile%d(0,%d)/%d to store", tile, th, tileIndex, curHashIdx)
		ts.Add(tile)
		tileIndex++
		hashes = make([][]byte, tileWidth)
	}

	// Build higher-level derived tiles.
	subtileCount := tileIndex
	subtileLevel := uint64(0)
	// If the layer below has more than one tile, or if it has exactly one complete tile, populate
	// the layer above.
	for subtileCount > 1 || curHashIdx == 0 {
		curHashIdx = 0
		tileIndex = 0
		for idx := uint64(0); idx < subtileCount; idx++ {
			subCoords := Coords{TileHeight: th, Level: subtileLevel, Index: idx}
			subTile, err := ts.TileAt(subCoords)
			if err != nil {
				t.Fatalf("TileAt(%v)=nil,%v; want _,nil", subCoords, err)
			}
			hash, err := subTile.RootHash()
			if err != nil {
				break
			}
			glog.V(2).Infof("store node %v in tile%d(%d,%d).Hashes[%d] = %x", subTile.Above(), th, subtileLevel+1, tileIndex, curHashIdx, hash[:8])
			hashes[curHashIdx] = hash
			curHashIdx++
			if curHashIdx == tileWidth {
				// Have a complete level l+1 tile, add it to the store.
				coords := Coords{TileHeight: th, Level: subtileLevel + 1, Index: tileIndex}
				tile, err := NewTile(coords, hashes, h2)
				if err != nil {
					t.Fatalf("NewTile(%v)=nil,%v; want _,nil", coords, err)
				}
				glog.V(2).Infof("add complete %v=tile%d(%d,%d) to store", tile, th, subtileLevel+1, tileIndex)
				ts.Add(tile)
				curHashIdx = 0
				tileIndex++
				hashes = make([][]byte, tileWidth)
			}
		}
		// There may be a final incomplete tile.
		if curHashIdx != 0 {
			coords := Coords{TileHeight: th, Level: subtileLevel + 1, Index: tileIndex}
			tile, err := NewTile(coords, hashes[:curHashIdx], h2)
			if err != nil {
				t.Fatalf("NewTile(%v)=nil,%v; want _,nil", coords, err)
			}
			glog.V(2).Infof("add incomplete %v=tile%d(%d,%d)/%d to store", tile, th, subtileLevel+1, tileIndex, curHashIdx)
			ts.Add(tile)
			tileIndex++
			hashes = make([][]byte, tileWidth)
		}

		// Up a level.
		subtileCount = tileIndex
		subtileLevel++
	}

	if got := uint64(imt.LeafCount()); got != treeSize {
		t.Fatalf("imt.LeafCount()=%d, want %d", got, treeSize)
	}
	return imt
}

func testHashFromStoreAll(t *testing.T, constructor storeConstructor) {
	heights := []uint64{2, 3, 4, 8}
	sizes := []uint64{2, 3, 5, 6, 7, 8, 9, 31, 32, 33, 34, 45, 127, 128, 129, 143}
	for _, tileHeight := range heights {
		for _, treeSize := range sizes {
			t.Run(fmt.Sprintf("tileHeight-%d-treeSize-%d", tileHeight, treeSize), func(t *testing.T) {
				ts := constructor(tileHeight, h2)
				buildTreeAndTiles(t, ts, treeSize)
				hashMap, desc := treeInfo(treeSize, 4)

				// Check every populated Merkle coordinate has the right hash.
				for coords, want := range hashMap {
					got, err := HashFromStore(ts, coords)
					if err != nil {
						t.Errorf("HashFromStore(%v)=nil,%v, want %x,nil", coords, err, want[:8])
						continue
					}
					if !bytes.Equal(got, want) {
						t.Errorf("HashFromStore(%v)=%x, want %x", coords, got[:8], want[:8])
						t.Logf("tree is:\n%s", desc)
					}
				}
			})
		}
	}
}

func testHashFromStoreErrors(t *testing.T, constructor storeConstructor) {
	tests := []struct {
		size   uint64
		coords MerkleCoords
		want   RequiredTilesError
	}{
		// Tiles completely absent from a tree of size 31.
		{size: 31, coords: mc(0, 32), want: RequiredTilesError{Missing: []MissingTileError{{Coords: tc2(0, 8)}}}},
		{size: 31, coords: mc(5, 2), want: RequiredTilesError{Missing: []MissingTileError{{Coords: tc2(2, 1)}}}},
		{size: 31, coords: mc(7, 0), want: RequiredTilesError{Missing: []MissingTileError{{Coords: tc2(3, 0)}}}},
		// Tiles that are incomplete in a tree of size 31.
		{size: 31, coords: mc(0, 31), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(0, 7), Index: 3, Size: 3}}}},
		{size: 31, coords: mc(1, 15), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(0, 7), Index: 3, Size: 3}}}},
		{size: 31, coords: mc(2, 7), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(1, 1), Index: 3, Size: 3}}}},
		{size: 31, coords: mc(3, 3), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(1, 1), Index: 3, Size: 3}}}},
		{size: 31, coords: mc(4, 1), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(2, 0), Index: 1, Size: 1}}}},
		{size: 31, coords: mc(4, 2), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(2, 0), Index: 2, Size: 1}}}},
		{size: 31, coords: mc(4, 3), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(2, 0), Index: 3, Size: 1}}}},
		{size: 31, coords: mc(5, 0), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(2, 0), Index: 1, Size: 1}}}},
		// Tiles that are incomplete in a tree of size 6.
		{size: 6, coords: mc(2, -7), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(0, 1), Index: 2, Size: 2}}}},
		{size: 6, coords: mc(3, 0), want: RequiredTilesError{Incomplete: []IncompleteTileError{{Coords: tc2(1, 0), Index: 1, Size: 1}}}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v-in-size-%d", test.coords, test.size), func(t *testing.T) {
			ts := constructor(2, h2)
			buildTreeAndTiles(t, ts, test.size)
			got, gotErr := HashFromStore(ts, test.coords)
			if gotErr == nil {
				t.Errorf("HashFromStore(%v)=%x,nil; want nil,'%v'", test.coords, got, test.want)
			}
			if !reflect.DeepEqual(gotErr, test.want) {
				t.Errorf("HashFromStore(%v)=nil,'%v' (%T); want nil,'%v' (%T)", test.coords, gotErr, gotErr, test.want, test.want)
			}
		})
	}
}

func testStoreInclusionProof(t *testing.T, constructor storeConstructor) {
	sizes := []uint64{8, 9, 31, 32, 33, 34, uint64(100 + rand.Int63n(100)), uint64(200 + rand.Int63n(200)), uint64(500 + rand.Int63n(500))}
	for _, treeSize := range sizes {
		t.Run(fmt.Sprintf("size-%d", treeSize), func(t *testing.T) {
			ts := constructor(2, h2)
			imt := buildTreeAndTiles(t, ts, treeSize)
			verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)

			trials := 100
			for i := 0; i < trials; i++ {
				idx := uint64(rand.Int63n(int64(treeSize - 1)))

				// Get the leaf hash from the tile store and from the in-memory tree, and check
				// they're the same.
				leafHash, err := HashFromStore(ts, MerkleCoords{Level: 0, Index: idx})
				if err != nil {
					t.Fatalf("failed to find leaf hash [%d]: %v", idx, err)
				}
				if want := imt.LeafHash(int64(idx + 1)); !bytes.Equal(leafHash, want) {
					t.Fatalf("leaf hash [%d]=%x in Store but =%x in imt", idx, leafHash, want)
				}

				// Get an inclusion proof for the chosen index from the tile store and from the
				// in-memory tree, and check they're the same.
				proof, err := InclusionProofFromStore(ts, idx, treeSize)
				if err != nil {
					t.Fatalf("InclusionProofFromStore(%d,%d)=nil,%v; want _,nil", idx, treeSize, err)
				}
				path := imt.PathToCurrentRoot(int64(idx + 1)) // need +1 for 1-based indexing
				var want [][]byte
				for _, entry := range path {
					want = append(want, entry.Value.Hash())
				}
				if !reflect.DeepEqual(proof, want) {
					t.Errorf("InclusionProofFromStore(%d,%d)=%x, want %x", idx, treeSize, proof, want)
				}

				// Calculate the root from the proof emitted by the tile store, and check it matches
				// the root from the in-memory tree.
				wantRoot, err := verifier.RootFromInclusionProof(int64(idx), int64(treeSize), proof, leafHash)
				if err != nil {
					t.Fatalf("failed to calculate root hash: %v", err)
				}
				gotRoot := imt.CurrentRoot().Hash()
				if !bytes.Equal(gotRoot, wantRoot) {
					t.Errorf("root from proof=%x, root from imt=%x", gotRoot, wantRoot)
				}

				// Asking for an inclusion proof to a larger tree size should fail.
				if proof, err := InclusionProofFromStore(ts, idx, treeSize+10); err == nil {
					t.Errorf("InclusionProofFromStore(%d,%d)=%x,nil; want nil, error", idx, treeSize+10, proof)
				}
			}
		})
	}
}

func testStoreConsistencyProof(t *testing.T, constructor storeConstructor) {
	tests := []struct {
		from, to uint64
	}{
		{from: 1, to: 2},
		{from: 2, to: 3},
		{from: 2, to: 5},
		{from: 5, to: 5},
		{from: 2, to: 8},
		{from: 4, to: 8},
		{from: 14, to: 18},
		{from: 14, to: 800},
		{from: 14, to: 8000},
		{from: 14, to: 80000},
		{from: 140, to: 80000},
		{from: 1401, to: 80000},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d-to-%d", test.from, test.to), func(t *testing.T) {
			ts := constructor(2, h2)
			imt := buildTreeAndTiles(t, ts, test.to)
			verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)

			proof, err := ConsistencyProofFromStore(ts, test.from, test.to)
			if err != nil {
				t.Fatalf("ConsistencyProofFromStore(%d,%d)=nil,%v; want _,nil", test.from, test.to, err)
			}
			hash1 := imt.RootAtSnapshot(int64(test.from)).Hash()
			hash2 := imt.CurrentRoot().Hash()

			if err := verifier.VerifyConsistencyProof(int64(test.from), int64(test.to), hash1, hash2, proof); err != nil {
				t.Fatalf("ConsistencyProofFromStore(%d,%d) fails to verify: %v", test.from, test.to, err)
			}
			path := imt.SnapshotConsistency(int64(test.from), int64(test.to))
			var want [][]byte
			for _, entry := range path {
				want = append(want, entry.Value.Hash())
			}
			if !reflect.DeepEqual(proof, want) {
				t.Errorf("ConsistencyProofFromStore(%d,%d)=%x, want %x", test.from, test.to, proof, want)
			}

			// Asking for a consistency proof to a larger tree size should fail.
			if proof, err := ConsistencyProofFromStore(ts, test.from, test.to+10); err == nil {
				t.Errorf("ConsistencyProofFromStore(%d,%d)=%x,nil; want nil, error", test.from, test.to+10, proof)
			}
		})
	}

}

// Tests using MemoryStore
func TestInvalidStoreHeight(t *testing.T) {
	testInvalidStoreHeight(t, NewMemoryStore)
}
func TestMemoryStoreSmall(t *testing.T) {
	testStoreSmall(t, NewMemoryStore)
}
func TestHashFromStoreAll(t *testing.T) {
	testHashFromStoreAll(t, NewMemoryStore)
}
func TestHashFromStoreErrors(t *testing.T) {
	testHashFromStoreErrors(t, NewMemoryStore)
}
func TestStoreInclusionProof(t *testing.T) {
	testStoreInclusionProof(t, NewMemoryStore)
}
func TestStoreConsistencyProof(t *testing.T) {
	testStoreConsistencyProof(t, NewMemoryStore)
}

package batchmap

import (
	"crypto"
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/trillian/experimental/batchmap/tilepb"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/ptest"
	"github.com/apache/beam/sdks/go/pkg/beam/transforms/filter"
	"github.com/apache/beam/sdks/go/pkg/beam/transforms/stats"
)

const hash = crypto.SHA512_256

func TestMain(m *testing.M) {
	ptest.Main(m)
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name         string
		prefixStrata int
		entries      []*tilepb.Entry
		treeID       int64
		hash         crypto.Hash

		wantRoot      string
		wantTileCount int

		wantFailConstruct bool
		wantFailRun       bool
	}{
		{
			name:          "single entry in one tile",
			prefixStrata:  0,
			entries:       []*tilepb.Entry{createEntry("ak", "av")},
			treeID:        12345,
			hash:          crypto.SHA512_256,
			wantRoot:      "af079c268bd48eb89532b2b0c96d753c8f98eb8ce03f5dd95fa60ab9cc92f3a4",
			wantTileCount: 1,
		},
		{
			name:          "single entry in one tile with different tree ID",
			prefixStrata:  0,
			entries:       []*tilepb.Entry{createEntry("ak", "av")},
			treeID:        54321,
			hash:          crypto.SHA512_256,
			wantRoot:      "8e6363380169b790b6e3d1890fc3d492a73512d9bbbfb886854e10ca10fc147f",
			wantTileCount: 1,
		},
		{
			name:          "single entry in stratified map",
			prefixStrata:  1,
			entries:       []*tilepb.Entry{createEntry("ak", "av")},
			treeID:        12345,
			hash:          crypto.SHA512_256,
			wantRoot:      "af079c268bd48eb89532b2b0c96d753c8f98eb8ce03f5dd95fa60ab9cc92f3a4",
			wantTileCount: 2,
		},
		{
			name:          "3 entries in one tile",
			prefixStrata:  0,
			entries:       []*tilepb.Entry{createEntry("ak", "av"), createEntry("bk", "bv"), createEntry("ck", "cv")},
			treeID:        12345,
			hash:          crypto.SHA512_256,
			wantRoot:      "2372f0432e04dc76015f427ce8a1294644e36421b047ddfd52afdfdba60aff25",
			wantTileCount: 1,
		},
		{
			name:          "3 entries in stratified map",
			prefixStrata:  1,
			entries:       []*tilepb.Entry{createEntry("ak", "av"), createEntry("bk", "bv"), createEntry("ck", "cv")},
			treeID:        12345,
			hash:          crypto.SHA512_256,
			wantRoot:      "2372f0432e04dc76015f427ce8a1294644e36421b047ddfd52afdfdba60aff25",
			wantTileCount: 4,
		},
		{
			name:         "duplicate keys",
			prefixStrata: 0,
			entries:      []*tilepb.Entry{createEntry("ak", "av"), createEntry("ak", "av")},
			treeID:       12345,
			hash:         crypto.SHA512_256,
			wantFailRun:  true,
		},
		{
			name:              "invalid prefixStrata (too small)",
			prefixStrata:      -1,
			entries:           []*tilepb.Entry{createEntry("ak", "av")},
			treeID:            12345,
			hash:              crypto.SHA512_256,
			wantFailConstruct: true,
		},
		{
			name:              "invalid prefixStrata (too large)",
			prefixStrata:      32,
			entries:           []*tilepb.Entry{createEntry("ak", "av")},
			treeID:            12345,
			hash:              crypto.SHA512_256,
			wantFailConstruct: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			// t.Parallel() would be great, but it seems Flume tests are sketchy about this?
			p, s := beam.NewPipelineWithRoot()
			leaves := beam.CreateList(s, test.entries)

			tiles, err := Create(s, leaves, test.treeID, test.hash, test.prefixStrata)
			if got, want := err != nil, test.wantFailConstruct; got != want {
				t.Errorf("pipeline construction failure: got %v, want %v (%v)", got, want, err)
			}
			if test.wantFailConstruct {
				return
			}
			rootTile := filter.Include(s, tiles, func(t *tilepb.Tile) bool { return len(t.GetPath()) == 0 })
			roots := beam.ParDo(s, func(t *tilepb.Tile) string { return fmt.Sprintf("%x", t.GetRootHash()) }, rootTile)

			assertTileCount(s, tiles, test.wantTileCount)
			passert.Equals(s, roots, test.wantRoot)
			err = ptest.Run(p)
			if got, want := err != nil, test.wantFailRun; got != want {
				t.Errorf("pipeline run failure: got %v, want %v (%v)", got, want, err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name                       string
		prefixStrata               int
		baseEntries, updateEntries []*tilepb.Entry
		treeID                     int64

		wantRoot      string
		wantTileCount int

		wantFailConstruct bool
		wantFailRun       bool
	}{
		{
			name:          "update single entry in single tile",
			prefixStrata:  0,
			baseEntries:   []*tilepb.Entry{createEntry("ak", "ignored")},
			updateEntries: []*tilepb.Entry{createEntry("ak", "av")},
			treeID:        12345,
			wantRoot:      "af079c268bd48eb89532b2b0c96d753c8f98eb8ce03f5dd95fa60ab9cc92f3a4",
			wantTileCount: 1,
		},
		{
			name:          "update single entry in stratified map",
			prefixStrata:  3,
			baseEntries:   []*tilepb.Entry{createEntry("ak", "ignored")},
			updateEntries: []*tilepb.Entry{createEntry("ak", "av")},
			treeID:        12345,
			wantRoot:      "af079c268bd48eb89532b2b0c96d753c8f98eb8ce03f5dd95fa60ab9cc92f3a4",
			wantTileCount: 4,
		},
		{
			name:          "3 entries in one tile",
			prefixStrata:  0,
			baseEntries:   []*tilepb.Entry{createEntry("ak", "ignored"), createEntry("bk", "bv")},
			updateEntries: []*tilepb.Entry{createEntry("ak", "av"), createEntry("ck", "cv")},
			treeID:        12345,
			wantRoot:      "2372f0432e04dc76015f427ce8a1294644e36421b047ddfd52afdfdba60aff25",
			wantTileCount: 1,
		},
		{
			name:          "3 entries in stratified map",
			prefixStrata:  3,
			baseEntries:   []*tilepb.Entry{createEntry("ak", "ignored"), createEntry("bk", "bv")},
			updateEntries: []*tilepb.Entry{createEntry("ak", "av"), createEntry("ck", "cv")},
			treeID:        12345,
			wantRoot:      "2372f0432e04dc76015f427ce8a1294644e36421b047ddfd52afdfdba60aff25",
			wantTileCount: 10,
		},
		{
			name:          "duplicate keys",
			prefixStrata:  3,
			baseEntries:   []*tilepb.Entry{createEntry("ak", "ignored")},
			updateEntries: []*tilepb.Entry{createEntry("ak", "av1"), createEntry("ak", "av2")},
			treeID:        12345,
			wantFailRun:   true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			p, s := beam.NewPipelineWithRoot()
			leaves := beam.CreateList(s, test.baseEntries)

			base, err := Create(s, leaves, test.treeID, hash, test.prefixStrata)
			if err != nil {
				t.Fatalf("failed to create pipeline: %v", err)
			}

			delta := beam.CreateList(s, test.updateEntries)
			tiles, err := Update(s, base, delta, test.treeID, hash, test.prefixStrata)
			if err != nil {
				t.Errorf("pipeline construction failure: %v", err)
			}
			rootTile := filter.Include(s, tiles, func(t *tilepb.Tile) bool { return len(t.GetPath()) == 0 })
			roots := beam.ParDo(s, func(t *tilepb.Tile) string { return fmt.Sprintf("%x", t.GetRootHash()) }, rootTile)

			assertTileCount(s, tiles, test.wantTileCount)
			passert.Equals(s, roots, test.wantRoot)
			err = ptest.Run(p)
			if got, want := err != nil, test.wantFailRun; got != want {
				t.Errorf("pipeline run failure: got %v, want %v (%v)", got, want, err)
			}
		})
	}
}

func TestChildrenSorted(t *testing.T) {
	p, s := beam.NewPipelineWithRoot()
	entries := []*tilepb.Entry{}
	for i := 0; i < 20; i++ {
		entries = append(entries, createEntry(fmt.Sprintf("key: %d", i), fmt.Sprintf("value: %d", i)))
	}

	tiles, err := Create(s, beam.CreateList(s, entries), 12345, hash, 1)
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}

	passert.True(s, tiles, func(t *tilepb.Tile) bool { return isStrictlySorted(t.GetLeaves()) })

	if err := ptest.Run(p); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
}

func TestGoldenCreate(t *testing.T) {
	p, s := beam.NewPipelineWithRoot()
	leaves := beam.CreateList(s, leafNodes(t, 500))

	tiles, err := Create(s, leaves, 42, crypto.SHA256, 3)
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}
	rootTile := filter.Include(s, tiles, func(t *tilepb.Tile) bool { return len(t.GetPath()) == 0 })
	roots := beam.ParDo(s, func(t *tilepb.Tile) string { return fmt.Sprintf("%x", t.GetRootHash()) }, rootTile)

	assertTileCount(s, tiles, 1218)
	passert.Equals(s, roots, "daf17dc2c83f37962bae8a65d294ef7fca4ffa02c10bdc4ca5c4dec408001c98")
	if err := ptest.Run(p); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
}

func TestGoldenUpdate(t *testing.T) {
	treeID := int64(42)
	strata := 3
	hash := crypto.SHA256

	p, s := beam.NewPipelineWithRoot()
	entries := leafNodes(t, 500)

	base, err := Create(s, beam.CreateList(s, entries[:300]), treeID, hash, strata)
	if err != nil {
		t.Fatalf("failed to create v0 pipeline: %v", err)
	}
	tiles, err := Update(s, base, beam.CreateList(s, entries[300:]), treeID, hash, strata)
	if err != nil {
		t.Fatalf("failed to create v1 pipeline: %v", err)
	}

	rootTile := filter.Include(s, tiles, func(t *tilepb.Tile) bool { return len(t.GetPath()) == 0 })
	roots := beam.ParDo(s, func(t *tilepb.Tile) string { return fmt.Sprintf("%x", t.GetRootHash()) }, rootTile)

	assertTileCount(s, tiles, 1218)
	passert.Equals(s, roots, "daf17dc2c83f37962bae8a65d294ef7fca4ffa02c10bdc4ca5c4dec408001c98")
	if err := ptest.Run(p); err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
}

// assertTileCount adds a check into the pipeline that the given PCollection of
// tiles has the given cardinality. If the check fails then ptest.Run will
// return an error.
func assertTileCount(s beam.Scope, tiles beam.PCollection, count int) {
	countTiles := func(t *tilepb.Tile) int { return 1 }
	passert.Equals(s, stats.Sum(s, beam.ParDo(s, countTiles, tiles)), count)
}

// Copied from http://google3/third_party/golang/trillian/merkle/smt/hstar3_test.go?l=201&rcl=298994396
func leafNodes(t testing.TB, n int) []*tilepb.Entry {
	t.Helper()
	// Use a random sequence that depends on n.
	r := rand.New(rand.NewSource(int64(n)))
	entries := make([]*tilepb.Entry, n)
	for i := range entries {
		value := make([]byte, 32)
		if _, err := r.Read(value); err != nil {
			t.Fatalf("Failed to make random leaf hash: %v", err)
		}
		path := make([]byte, 32)
		if _, err := r.Read(path); err != nil {
			t.Fatalf("Failed to make random path: %v", err)
		}
		entries[i] = &tilepb.Entry{
			HashKey:   path,
			HashValue: value,
		}
	}

	return entries
}

func createEntry(k, v string) *tilepb.Entry {
	h := crypto.SHA256.New()
	h.Write([]byte(k))
	hk := h.Sum(nil)

	h = crypto.SHA256.New()
	h.Write([]byte(v))
	hv := h.Sum(nil)

	return &tilepb.Entry{
		HashKey:   hk,
		HashValue: hv,
	}
}

func isStrictlySorted(leaves []*tilepb.TileLeaf) bool {
	for i := 1; i < len(leaves); i++ {
		lPath, rPath := leaves[i-1].GetPath(), leaves[i].GetPath()
		if string(lPath) >= string(rPath) {
			return false
		}
	}
	return true
}

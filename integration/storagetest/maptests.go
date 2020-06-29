// Copyright 2019 Google LLC. All Rights Reserved.
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

// Package storagetest verifies that storage interfaces behave correctly
package storagetest

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/types"

	storageto "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
)

// MapStorageFactory creates MapStorage and AdminStorage for a test to use.
type MapStorageFactory = func(ctx context.Context, t *testing.T) (storage.MapStorage, storage.AdminStorage)

// MapStorageTest executes a test using the given storage implementations.
type MapStorageTest = func(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage)

// RunMapStorageTests runs all the map storage tests against the provided map storage implementation.
func RunMapStorageTests(t *testing.T, storageFactory MapStorageFactory) {
	ctx := context.Background()
	for name, f := range mapTestFunctions(t, &mapTests{}) {
		ms, as := storageFactory(ctx, t)
		t.Run(name, func(t *testing.T) { f(ctx, t, ms, as) })
	}
}

func mapTestFunctions(t *testing.T, x interface{}) map[string]MapStorageTest {
	tests := make(map[string]MapStorageTest)
	xv := reflect.ValueOf(x)
	for _, name := range testFunctions(x) {
		m := xv.MethodByName(name)
		if !m.IsValid() {
			t.Fatalf("storagetest: function %v is not valid", name)
		}
		f, ok := m.Interface().(MapStorageTest)
		if !ok {
			// Method exists but has the wrong type signature.
			t.Fatalf("storagetest: function %v has unexpected signature (%T), want %v", name, m.Interface(), m)
		}
		nickname := strings.TrimPrefix(name, "Test")
		tests[nickname] = f
	}
	return tests
}

// MapTests is a suite of tests to run against the storage.MapTest interface.
type mapTests struct{}

// TestCheckDatabaseAccessible fails the test if the map storage is not accessible.
func (*mapTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.MapStorage, _ storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

// TestMapSnapshot fails the test if MapStorage.SnapshotForTree() does not behave correctly.
func (*mapTests) TestMapSnapshot(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage) {
	frozenMap := createInitializedMapForTests(ctx, t, s, as)
	storage.UpdateTree(ctx, as, frozenMap.TreeId, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	})

	activeMap := createInitializedMapForTests(ctx, t, s, as)
	logID := mustCreateTree(ctx, t, as, storageto.LogTree).TreeId

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "unknownSnapshot",
			tree:    mapTree(-1),
			wantErr: true,
		},
		{
			desc: "activeMapSnapshot",
			tree: activeMap,
		},
		{
			desc: "frozenSnapshot",
			tree: frozenMap,
		},
		{
			desc:    "logSnapshot",
			tree:    mapTree(logID),
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tx, err := s.SnapshotForTree(ctx, test.tree)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("SnapshotForTree(%v): %v; want err: %v", test.tree.TreeId, err, test.wantErr)
			}
			if err != nil {
				return
			}
			defer tx.Close()

			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit()=_,%v; want _,nil", err)
			}
		})
	}
}

func (*mapTests) TestMapLayout(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage) {
	tree := createInitializedMapForTests(ctx, t, s, as)
	if _, err := s.Layout(tree); err != nil {
		t.Errorf("Layout(): %v", err)
	}
}

func (*mapTests) TestGetTile(ctx context.Context, t *testing.T, ms storage.MapStorage, as storage.AdminStorage) {
	maptree := createInitializedMapForTests(ctx, t, ms, as)

	numTiles := 10
	tiles := []smt.Tile{}
	for i := 0; i < numTiles; i++ {
		nodes, err := smt.NewNodesRow([]smt.Node{{
			ID:   tree.NewNodeID2(fmt.Sprintf("%d1", i), 16),
			Hash: []byte{1},
		}})
		if err != nil {
			t.Fatalf("NewNodesRow(): %v", err)
		}
		tiles = append(tiles, smt.Tile{ID: tree.NewNodeID2(fmt.Sprintf("%d", i), 8), Leaves: nodes})
	}

	if err := ms.ReadWriteTransaction(ctx, maptree, func(ctx context.Context, tx storage.MapTreeTX) error {
		if err := tx.SetTiles(ctx, tiles); err != nil {
			t.Fatalf("SetTiles(): %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("ReadWriteTransaction(): %v", err)
	}
	mustSignAndStoreMapRoot(ctx, t, ms, maptree, &types.MapRootV1{Revision: 1, TimestampNanos: 1})

	ids := []tree.NodeID2{}
	for _, tile := range tiles {
		ids = append(ids, tile.ID)
	}

	if err := ms.ReadWriteTransaction(ctx, maptree, func(ctx context.Context, tx storage.MapTreeTX) error {
		got, err := tx.GetTiles(ctx, 1, ids)
		if err != nil {
			t.Fatalf("GetTiles(): %v", err)
		}
		opts := []cmp.Option{
			cmp.Comparer(func(x, y tree.NodeID2) bool { return x.String() == y.String() }),
			cmpopts.SortSlices(func(x, y smt.Tile) bool { return strings.Compare(x.ID.String(), y.ID.String()) < 0 }),
		}
		if !cmp.Equal(got, tiles, opts...) {
			t.Fatalf("GetTiles(): diff: %v", cmp.Diff(got, tiles, opts...))
		}
		return nil
	}); err != nil {
		t.Fatalf("ReadWriteTransaction(): %v", err)
	}

}

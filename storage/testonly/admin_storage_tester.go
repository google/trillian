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

package testonly

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
)

var (
	// LogTree is a valid, LOG-type trillian.Tree for tests.
	LogTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC_6962,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DuplicatePolicy:    trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
	}

	// MapTree is a valid, MAP-type trillian.Tree for tests.
	MapTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_MAP,
		HashStrategy:       trillian.HashStrategy_RFC_6962,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DuplicatePolicy:    trillian.DuplicatePolicy_DUPLICATES_ALLOWED,
		DisplayName:        "Llamas Map",
		Description:        "Key Transparency map for all your digital llama needs.",
	}
)

// AdminStorageTester runs a suite of tests against AdminStorage implementations.
type AdminStorageTester struct {
	// NewAdminStorage returns an AdminStorage instance pointing to a clean
	// test database.
	NewAdminStorage func() storage.AdminStorage
}

// RunAllTests runs all AdminStorage tests.
func (tester *AdminStorageTester) RunAllTests(t *testing.T) {
	t.Run("TestCreateTree", tester.TestCreateTree)
	t.Run("TestUpdateTree", tester.TestUpdateTree)
	t.Run("TestListTrees", tester.TestListTrees)
	t.Run("TestAdminTXClose", tester.TestAdminTXClose)
}

// TestCreateTree tests AdminStorage Tree creation.
func (tester *AdminStorageTester) TestCreateTree(t *testing.T) {
	// Check that validation runs, but leave details to the validation
	// tests.
	invalidTree := *LogTree
	invalidTree.TreeType = trillian.TreeType_UNKNOWN_TREE_TYPE

	validTree1 := *LogTree
	validTree2 := *MapTree

	tests := []struct {
		tree    *trillian.Tree
		wantErr bool
	}{
		{tree: &invalidTree, wantErr: true},
		{tree: &validTree1},
		{tree: &validTree2},
	}

	s := tester.NewAdminStorage()
	for i, test := range tests {
		ctx := context.Background()
		tx, err := s.Begin(ctx)
		if err != nil {
			t.Fatalf("%v: Begin() = %v, want = nil", i, err)
		}
		defer tx.Close()

		// Test CreateTree up to the tx commit
		newTree, err := tx.CreateTree(ctx, test.tree)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", i, err, test.wantErr)
			continue
		} else if hasErr {
			// Tested above
			continue
		}
		switch {
		case newTree.TreeId == 0:
			t.Errorf("%v: TreeID not returned from creation: %v", i, newTree)
			continue
		case newTree.CreateTimeMillisSinceEpoch <= 0:
			t.Errorf("%v: CreateTime not returned from creation: %v", i, newTree)
			continue
		case newTree.CreateTimeMillisSinceEpoch != newTree.UpdateTimeMillisSinceEpoch:
			t.Errorf("%v: CreateTime != UpdateTime: %v", i, newTree)
			continue
		}
		wantTree := *test.tree
		wantTree.TreeId = newTree.TreeId
		wantTree.CreateTimeMillisSinceEpoch = newTree.CreateTimeMillisSinceEpoch
		wantTree.UpdateTimeMillisSinceEpoch = newTree.UpdateTimeMillisSinceEpoch
		if !reflect.DeepEqual(newTree, &wantTree) {
			t.Errorf("%v: newTree = %v, want = %v", i, newTree, wantTree)
			continue
		}
		if err := tx.Commit(); err != nil {
			t.Errorf("%v: Commit() = %v, want = nil", i, err)
			continue
		}

		// Make sure a tree was correctly stored
		storedTree, err := getTree(s, newTree.TreeId)
		if err != nil {
			t.Errorf(":%v: getTree() = (%v, %v), want = (%v, nil)", i, newTree.TreeId, err, newTree)
			continue
		}
		wantTree = *storedTree
		if !reflect.DeepEqual(newTree, &wantTree) {
			t.Errorf("%v: newTree = \n%v, wantTree = \n%v", i, newTree, &wantTree)
		}
	}
}

// TestUpdateTree tests AdminStorage Tree updates.
func (tester *AdminStorageTester) TestUpdateTree(t *testing.T) {
	s := tester.NewAdminStorage()

	unrelatedTree, err := createTree(s, MapTree)
	if err != nil {
		t.Fatalf("createTree() = (_, %v), want = (_, nil)", err)
	}

	referenceLog := *LogTree

	validLog := referenceLog
	validLog.TreeState = trillian.TreeState_FROZEN
	validLog.DisplayName = "Frozen Tree"
	validLog.Description = "A Frozen Tree"
	validLogFunc := func(t *trillian.Tree) {
		t.TreeState = validLog.TreeState
		t.DisplayName = validLog.DisplayName
		t.Description = validLog.Description
	}

	invalidLogFunc := func(t *trillian.Tree) {
		t.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE
	}

	readonlyChangedFunc := func(t *trillian.Tree) {
		t.TreeType = trillian.TreeType_MAP
	}

	referenceMap := *MapTree
	validMap := referenceMap
	validMap.DisplayName = "Updated Map"
	validMapFunc := func(t *trillian.Tree) {
		t.DisplayName = validMap.DisplayName
	}

	// Test for an unknown tree outside the loop: it makes the test logic simpler
	if _, errOnUpdate, err := updateTree(s, -1, func(t *trillian.Tree) {}); err == nil || !errOnUpdate {
		t.Errorf("updateTree() = (_, %v, %v), wanted lookup error (ie, error on update)", errOnUpdate, err)
	}

	tests := []struct {
		desc                 string
		createTree, wantTree *trillian.Tree
		updateFunc           func(*trillian.Tree)
		wantErr              bool
	}{
		{
			desc:       "validLog",
			createTree: &referenceLog,
			updateFunc: validLogFunc,
			wantTree:   &validLog,
		},
		{
			desc:       "invalidLog",
			createTree: &referenceLog,
			updateFunc: invalidLogFunc,
			wantErr:    true,
		},
		{
			desc:       "readonlyChanged",
			createTree: &referenceLog,
			updateFunc: readonlyChangedFunc,
			wantErr:    true,
		},
		{
			desc:       "validMap",
			createTree: &referenceMap,
			updateFunc: validMapFunc,
			wantTree:   &validMap,
		},
	}
	for _, test := range tests {
		createdTree, err := createTree(s, test.createTree)
		if err != nil {
			t.Errorf("createTree() = (_, %v), want = (_, nil)", err)
			continue
		}

		updatedTree, errOnUpdate, err := updateTree(s, createdTree.TreeId, test.updateFunc)
		if err != nil && !errOnUpdate {
			t.Errorf("%v: updateTree() failed with non-Update error: %v", test.desc, err)
			continue
		}

		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: updateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if createdTree.TreeId != updatedTree.TreeId {
			t.Errorf("%v: TreeId = %v, want = %v", test.desc, updatedTree.TreeId, createdTree.TreeId)
		}
		if createdTree.CreateTimeMillisSinceEpoch != updatedTree.CreateTimeMillisSinceEpoch {
			t.Errorf("%v: CreateTime = %v, want = %v", test.desc, updatedTree.CreateTimeMillisSinceEpoch, createdTree.CreateTimeMillisSinceEpoch)
		}
		if createdTree.UpdateTimeMillisSinceEpoch > updatedTree.UpdateTimeMillisSinceEpoch {
			t.Errorf("%v: UpdateTime = %v, want >= %v", test.desc, updatedTree.UpdateTimeMillisSinceEpoch, createdTree.UpdateTimeMillisSinceEpoch)
		}
		// Copy storage-generated values to wantTree before comparing
		wantTree := *test.wantTree
		wantTree.TreeId = updatedTree.TreeId
		wantTree.CreateTimeMillisSinceEpoch = updatedTree.CreateTimeMillisSinceEpoch
		wantTree.UpdateTimeMillisSinceEpoch = updatedTree.UpdateTimeMillisSinceEpoch
		if !reflect.DeepEqual(updatedTree, &wantTree) {
			t.Errorf("%v: updatedTree doesn't match wantTree:\n"+
				"got =  %v,\n"+
				"want = %v", test.desc, updatedTree, &wantTree)
		}

		if storedTree, err := getTree(s, updatedTree.TreeId); err != nil {
			t.Errorf(":%v: getTree() = (_, %v), want = (_, nil)", test.desc, err)
		} else if !reflect.DeepEqual(storedTree, updatedTree) {
			t.Errorf("%v: storedTree doesn't match updatedTree:\n"+
				"got =  %v,\n"+
				"want = %v", test.desc, storedTree, updatedTree)
		}

		if unrelatedAfterTest, err := getTree(s, unrelatedTree.TreeId); err != nil {
			t.Errorf("%v: getTree() = (_, %v), want = (_, nil)", test.desc, err)
		} else if !reflect.DeepEqual(unrelatedAfterTest, unrelatedTree) {
			t.Errorf("%v: unrelatedTree changed:\n"+
				"got  = %v,\n"+
				"want = %v", test.desc, unrelatedAfterTest, unrelatedTree)
		}
	}
}

func createTree(s storage.AdminStorage, tree *trillian.Tree) (*trillian.Tree, error) {
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	newTree, err := tx.CreateTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newTree, nil
}

// updateTree updates the specified tree.
// The bool return signifies whether the error was returned by the UpdateTree() call.
func updateTree(s storage.AdminStorage, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, bool, error) {
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Close()
	newTree, err := tx.UpdateTree(ctx, treeID, updateFunc)
	if err != nil {
		return nil, true, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return newTree, false, nil
}

func getTree(s storage.AdminStorage, treeID int64) (*trillian.Tree, error) {
	ctx := context.Background()
	tx, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	tree, err := tx.GetTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tree, nil
}

// TestListTrees tests both ListTreeIDs and ListTrees.
func (tester *AdminStorageTester) TestListTrees(t *testing.T) {
	tests := []struct {
		newTrees int
	}{
		{newTrees: 0},
		{newTrees: 1}, // total 1
		{newTrees: 3}, // total 4
	}

	s := tester.NewAdminStorage()

	wantTrees := []*trillian.Tree{}
	for i, test := range tests {
		// Setup new trees (as specified)
		if test.newTrees > 0 {
			before := len(wantTrees)
			ctx := context.Background()
			tx, err := s.Begin(ctx)
			if err != nil {
				t.Fatalf("%v: Begin() = %v, want = nil", i, err)
			}
			defer tx.Close()
			for j := 0; j < test.newTrees; j++ {
				tree, err := tx.CreateTree(ctx, LogTree)
				if err != nil {
					t.Fatalf("%v: CreateTree() = (_, %v), want = (_, nil)", i, err)
				}
				wantTrees = append(wantTrees, tree)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("%v: Commit() = %v, want = nil", i, err)
			}
			if got := len(wantTrees) - before; got != test.newTrees {
				t.Fatalf("got %v new trees, want = %v", got, test.newTrees)
			}
		}

		ctx := context.Background()
		tx, err := s.Snapshot(ctx)
		if err != nil {
			t.Fatalf("%v: Snapshot() = %v, want = nil", i, err)
		}
		defer tx.Close()
		runListTreeIDsTest(ctx, t, i, tx, wantTrees)
		runListTreesTest(ctx, t, i, tx, wantTrees)
		if err := tx.Commit(); err != nil {
			t.Errorf("%v: Commit() = %v, want = nil", i, err)
		}
	}
}

func runListTreeIDsTest(ctx context.Context, t *testing.T, i int, tx storage.ReadOnlyAdminTX, wantTrees []*trillian.Tree) {
	ids, err := tx.ListTreeIDs(ctx)
	if err != nil {
		t.Errorf("%v: ListTreeIDs() = (_, %v), want = (_, nil)", i, err)
		return
	}
	if got, want := len(ids), len(wantTrees); got != want {
		t.Errorf("%v: got len(ids) = %v, want = %v", i, got, want)
		return
	}
	wantIDs := []int64{}
	for _, tree := range wantTrees {
		wantIDs = append(wantIDs, tree.TreeId)
	}
	if got, want := toIntMap(ids), toIntMap(wantIDs); !reflect.DeepEqual(got, want) {
		t.Errorf("%v: ListTreeIDs = (%v, _), want = (%v, _)", i, ids, wantIDs)
		return
	}
}

func runListTreesTest(ctx context.Context, t *testing.T, i int, tx storage.ReadOnlyAdminTX, wantTrees []*trillian.Tree) {
	trees, err := tx.ListTrees(ctx)
	if err != nil {
		t.Errorf("%v: ListTrees() = (_, %v), want = (_, nil)", i, err)
		return
	}
	if got, want := len(trees), len(wantTrees); got != want {
		t.Errorf("%v: got len(ids) = %v, want = %v", i, got, want)
		return
	}
	if got, want := toTreeMap(trees), toTreeMap(wantTrees); !reflect.DeepEqual(got, want) {
		t.Errorf("%v: ListTrees = (%v, _), want = (%v, _)", i, trees, wantTrees)
		return
	}
}

func toIntMap(values []int64) map[int64]bool {
	m := make(map[int64]bool)
	for _, v := range values {
		m[v] = true
	}
	return m
}

func toTreeMap(values []*trillian.Tree) map[int64]*trillian.Tree {
	m := make(map[int64]*trillian.Tree)
	for _, v := range values {
		m[v.TreeId] = v
	}
	return m
}

// TestAdminTXClose verifies the behavior of Close() with and without explicit Commit() / Rollback() calls.
func (tester *AdminStorageTester) TestAdminTXClose(t *testing.T) {
	tests := []struct {
		commit       bool
		rollback     bool
		wantRollback bool
	}{
		{commit: true, wantRollback: false},
		{rollback: true, wantRollback: true},
		{wantRollback: true}, // Close() before Commit() or Rollback() will cause a rollback
	}

	s := tester.NewAdminStorage()
	for i, test := range tests {
		ctx := context.Background()
		tx, err := s.Begin(ctx)
		if err != nil {
			t.Fatalf("%v: Begin() = (_, %v), want = (_, nil)", i, err)
		}
		defer tx.Close()

		tree, err := tx.CreateTree(ctx, LogTree)
		if err != nil {
			t.Fatalf("%v: CreateTree() = (_, %v), want = (_, nil)", i, err)
		}

		if test.commit {
			if err := tx.Commit(); err != nil {
				t.Errorf("%v: Commit() = %v, want = nil", i, err)
				continue
			}
		}
		if test.rollback {
			if err := tx.Rollback(); err != nil {
				t.Errorf("%v: Rollback() = %v, want = nil", i, err)
				continue
			}
		}

		if err := tx.Close(); err != nil {
			t.Errorf("%v: Close() = %v, want = nil", i, err)
			continue
		}

		ctx = context.Background()
		tx2, err := s.Snapshot(ctx)
		if err != nil {
			t.Fatalf("%v: Snapshot() = (_, %v), want = (_, nil)", i, err)
		}
		defer tx2.Close()
		_, err = tx2.GetTree(ctx, tree.TreeId)
		if hasErr := err != nil; test.wantRollback != hasErr {
			t.Errorf("%v: GetTree() = (_, %v), but wantRollback = %v", i, err, test.wantRollback)
		}

		// Multiple Close() calls are fine too
		if err := tx.Close(); err != nil {
			t.Errorf("%v: Close() = %v, want = nil", i, err)
			continue
		}
	}
}

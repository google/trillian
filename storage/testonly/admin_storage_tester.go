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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	ktestonly "github.com/google/trillian/crypto/keys/testonly"
	"github.com/google/trillian/crypto/keyspb"
	spb "github.com/google/trillian/crypto/sigpb"
	_ "github.com/google/trillian/merkle/maphasher" // TESET_MAP_HASHER
	"github.com/google/trillian/storage"
	ttestonly "github.com/google/trillian/testonly"
	"github.com/kylelemons/godebug/pretty"
)

// mustMarshalAny panics if ptypes.MarshalAny fails.
func mustMarshalAny(pb proto.Message) *any.Any {
	value, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return value
}

var (
	// LogTree is a valid, LOG-type trillian.Tree for tests.
	LogTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(ttestonly.DemoPrivateKey, ttestonly.DemoPrivateKeyPass),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(ttestonly.DemoPublicKey),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}

	// MapTree is a valid, MAP-type trillian.Tree for tests.
	MapTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_MAP,
		HashStrategy:       trillian.HashStrategy_TEST_MAP_HASHER,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Map",
		Description:        "Key Transparency map for all your digital llama needs.",
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(ttestonly.DemoPrivateKey, ttestonly.DemoPrivateKeyPass),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(ttestonly.DemoPublicKey),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
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

	validTreeWithoutOptionals := *LogTree
	validTreeWithoutOptionals.DisplayName = ""
	validTreeWithoutOptionals.Description = ""

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "invalidTree",
			tree:    &invalidTree,
			wantErr: true,
		},
		{
			desc: "validTree1",
			tree: &validTree1,
		},
		{
			desc: "validTree2",
			tree: &validTree2,
		},
		{
			desc: "validTreeWithoutOptionals",
			tree: &validTreeWithoutOptionals,
		},
	}

	ctx := context.Background()
	s := tester.NewAdminStorage()
	for _, test := range tests {
		func() {
			tx, err := s.Begin(ctx)
			if err != nil {
				t.Fatalf("%v: Begin() = %v, want = nil", test.desc, err)
			}
			defer tx.Close()

			// Test CreateTree up to the tx commit
			newTree, err := tx.CreateTree(ctx, test.tree)
			if hasErr := err != nil; hasErr != test.wantErr {
				t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
				return
			} else if hasErr {
				// Tested above
				return
			}
			createTime := newTree.CreateTime
			updateTime := newTree.UpdateTime
			if _, err := ptypes.Timestamp(createTime); err != nil {
				t.Errorf("%v: CreateTime malformed after creation: %v", test.desc, newTree)
				return
			}
			switch {
			case newTree.TreeId == 0:
				t.Errorf("%v: TreeID not returned from creation: %v", test.desc, newTree)
				return
			case !reflect.DeepEqual(createTime, updateTime):
				t.Errorf("%v: CreateTime != UpdateTime: %v", test.desc, newTree)
				return
			}
			wantTree := *test.tree
			wantTree.TreeId = newTree.TreeId
			wantTree.CreateTime = createTime
			wantTree.UpdateTime = updateTime
			// Ignore storage_settings changes (OK to vary between implementations)
			wantTree.StorageSettings = newTree.StorageSettings
			if !proto.Equal(newTree, &wantTree) {
				diff := pretty.Compare(newTree, &wantTree)
				t.Errorf("%v: post-CreateTree diff:\n%v", test.desc, diff)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("%v: Commit() = %v, want = nil", test.desc, err)
				return
			}

			// Make sure a tree was correctly stored.
			storedTree, err := getTree(ctx, s, newTree.TreeId)
			if err != nil {
				t.Errorf(":%v: getTree() = (%v, %v), want = (%v, nil)", test.desc, newTree.TreeId, err, newTree)
				return
			}
			if !proto.Equal(storedTree, newTree) {
				diff := pretty.Compare(storedTree, newTree)
				t.Errorf("%v: storedTree differs:\n%s", test.desc, diff)
			}
		}()
	}
}

// TestUpdateTree tests AdminStorage Tree updates.
func (tester *AdminStorageTester) TestUpdateTree(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	unrelatedTree, err := createTree(ctx, s, MapTree)
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

	validLogWithoutOptionalsFunc := func(t *trillian.Tree) {
		t.DisplayName = ""
		t.Description = ""
	}
	validLogWithoutOptionals := referenceLog
	validLogWithoutOptionalsFunc(&validLogWithoutOptionals)

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
	if _, errOnUpdate, err := updateTree(ctx, s, -1, func(t *trillian.Tree) {}); err == nil || !errOnUpdate {
		t.Errorf("updateTree(_, -1, _) = (_, %v, %v), want = (_, true, lookup error)", errOnUpdate, err)
	}

	tests := []struct {
		desc         string
		create, want *trillian.Tree
		updateFunc   func(*trillian.Tree)
		wantErr      bool
	}{
		{
			desc:       "validLog",
			create:     &referenceLog,
			updateFunc: validLogFunc,
			want:       &validLog,
		},
		{
			desc:       "validLogWithoutOptionals",
			create:     &referenceLog,
			updateFunc: validLogWithoutOptionalsFunc,
			want:       &validLogWithoutOptionals,
		},
		{
			desc:       "invalidLog",
			create:     &referenceLog,
			updateFunc: invalidLogFunc,
			wantErr:    true,
		},
		{
			desc:       "readonlyChanged",
			create:     &referenceLog,
			updateFunc: readonlyChangedFunc,
			wantErr:    true,
		},
		{
			desc:       "validMap",
			create:     &referenceMap,
			updateFunc: validMapFunc,
			want:       &validMap,
		},
	}
	for _, test := range tests {
		createdTree, err := createTree(ctx, s, test.create)
		if err != nil {
			t.Errorf("createTree() = (_, %v), want = (_, nil)", err)
			continue
		}

		updatedTree, errOnUpdate, err := updateTree(ctx, s, createdTree.TreeId, test.updateFunc)
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
		if !reflect.DeepEqual(createdTree.CreateTime, updatedTree.CreateTime) {
			t.Errorf("%v: CreateTime = %v, want = %v", test.desc, updatedTree.CreateTime, createdTree.CreateTime)
		}
		createUpdateTime, err := ptypes.Timestamp(createdTree.UpdateTime)
		if err != nil {
			t.Errorf("%v: createdTree.UpdateTime malformed: %v", test.desc, err)
		}
		updatedUpdateTime, err := ptypes.Timestamp(updatedTree.UpdateTime)
		if err != nil {
			t.Errorf("%v: updatedTree.UpdateTime malformed: %v", test.desc, err)
		}
		if createUpdateTime.After(updatedUpdateTime) {
			t.Errorf("%v: UpdateTime = %v, want >= %v", test.desc, updatedTree.UpdateTime, createdTree.UpdateTime)
		}
		// Copy storage-generated values to want before comparing
		wantTree := *test.want
		wantTree.TreeId = updatedTree.TreeId
		wantTree.CreateTime = updatedTree.CreateTime
		wantTree.UpdateTime = updatedTree.UpdateTime
		// Ignore storage_settings changes (OK to vary between implementations)
		wantTree.StorageSettings = updatedTree.StorageSettings
		if !proto.Equal(updatedTree, &wantTree) {
			diff := pretty.Compare(updatedTree, &wantTree)
			t.Errorf("%v: updatedTree doesn't match wantTree:\n%s", test.desc, diff)
		}

		if storedTree, err := getTree(ctx, s, updatedTree.TreeId); err != nil {
			t.Errorf(":%v: getTree() = (_, %v), want = (_, nil)", test.desc, err)
		} else if !proto.Equal(storedTree, updatedTree) {
			diff := pretty.Compare(storedTree, updatedTree)
			t.Errorf("%v: storedTree doesn't match updatedTree:\n%s", test.desc, diff)
		}

		if unrelatedAfterTest, err := getTree(ctx, s, unrelatedTree.TreeId); err != nil {
			t.Errorf("%v: getTree() = (_, %v), want = (_, nil)", test.desc, err)
		} else if !proto.Equal(unrelatedAfterTest, unrelatedTree) {
			diff := pretty.Compare(unrelatedAfterTest, unrelatedTree)
			t.Errorf("%v: unrelatedTree changed:\n%s", test.desc, diff)
		}
	}
}

func createTree(ctx context.Context, s storage.AdminStorage, tree *trillian.Tree) (*trillian.Tree, error) {
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
func updateTree(ctx context.Context, s storage.AdminStorage, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, bool, error) {
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

func getTree(ctx context.Context, s storage.AdminStorage, treeID int64) (*trillian.Tree, error) {
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
		desc string
		// newTrees is the number of trees created before each test. The total of trees is
		// cumulative between tests.
		newTrees int
	}{
		{desc: "zero", newTrees: 0},
		{desc: "one", newTrees: 1},  // total 1
		{desc: "four", newTrees: 3}, // total 4
	}

	ctx := context.Background()
	s := tester.NewAdminStorage()

	wantTrees := []*trillian.Tree{}
	for _, test := range tests {
		func() {
			// Setup new trees (as specified)
			if test.newTrees > 0 {
				before := len(wantTrees)
				for j := 0; j < test.newTrees; j++ {
					tree, err := createTree(ctx, s, LogTree)
					if err != nil {
						t.Fatalf("%v: CreateTree() = (_, %v), want = (_, nil)", test.desc, err)
					}
					wantTrees = append(wantTrees, tree)
				}
				if got := len(wantTrees) - before; got != test.newTrees {
					t.Fatalf("%v: got %v new trees, want = %v", test.desc, got, test.newTrees)
				}
			}

			tx, err := s.Snapshot(ctx)
			if err != nil {
				t.Fatalf("%v: Snapshot() = %v, want = nil", test.desc, err)
			}
			defer tx.Close()
			if err := runListTreeIDsTest(ctx, tx, wantTrees); err != nil {
				t.Errorf("%v: %v", test.desc, err)
			}
			if err := runListTreesTest(ctx, tx, wantTrees); err != nil {
				t.Errorf("%v: %v", test.desc, err)
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("%v: Commit() = %v, want = nil", test.desc, err)
			}
		}()
	}
}

func runListTreeIDsTest(ctx context.Context, tx storage.ReadOnlyAdminTX, wantTrees []*trillian.Tree) error {
	ids, err := tx.ListTreeIDs(ctx)
	if err != nil {
		return fmt.Errorf("ListTreeIDs() = (_, %v), want = (_, nil)", err)
	}
	if got, want := len(ids), len(wantTrees); got != want {
		return fmt.Errorf("got len(ids) = %v, want = %v", got, want)
	}
	wantIDs := []int64{}
	for _, tree := range wantTrees {
		wantIDs = append(wantIDs, tree.TreeId)
	}
	got, want := toIntMap(ids), toIntMap(wantIDs)
	if diff := pretty.Compare(got, want); diff != "" {
		return fmt.Errorf("post-ListTreeIDs() diff:\n%v", diff)
	}
	return nil
}

func runListTreesTest(ctx context.Context, tx storage.ReadOnlyAdminTX, wantTrees []*trillian.Tree) error {
	trees, err := tx.ListTrees(ctx)
	if err != nil {
		return fmt.Errorf("ListTrees() = (_, %v), want = (_, nil)", err)
	}
	if got, want := len(trees), len(wantTrees); got != want {
		return fmt.Errorf("got len(ids) = %v, want = %v", got, want)
	}
	treesMap, wantMap := toTreeMap(trees), toTreeMap(wantTrees)
	if len(trees) != len(treesMap) {
		return fmt.Errorf("found duplicate on trees: %v", trees)
	}
	if len(wantTrees) != len(wantMap) {
		return fmt.Errorf("found duplicate on wantTrees: %v", wantTrees)
	}
	for _, want := range wantTrees {
		found := false
		for _, tree := range trees {
			if proto.Equal(tree, want) {
				found = true
				break
			}
		}
		if !found {
			diff := pretty.Compare(treesMap, wantMap)
			return fmt.Errorf("post-ListTrees diff:\n%v", diff)
		}
	}
	return nil
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

	ctx := context.Background()
	s := tester.NewAdminStorage()

	for i, test := range tests {
		func() {
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
					return
				}
			}
			if test.rollback {
				if err := tx.Rollback(); err != nil {
					t.Errorf("%v: Rollback() = %v, want = nil", i, err)
					return
				}
			}

			if err := tx.Close(); err != nil {
				t.Errorf("%v: Close() = %v, want = nil", i, err)
				return
			}

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
				return
			}
		}()
	}
}

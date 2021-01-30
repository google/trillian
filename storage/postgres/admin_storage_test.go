// Copyright 2018 Google LLC. All Rights Reserved.
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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
)

var (
	allTables = []string{"unsequenced", "tree_head", "sequenced_leaf_data", "leaf_data", "subtree", "tree_control", "trees"}
	db        *sql.DB
)

const selectTreeControlByID = "SELECT signing_enabled, sequencing_enabled, sequence_interval_seconds FROM tree_control WHERE tree_id = $1"

func TestPgAdminStorage(t *testing.T) {
	tester := &testonly.AdminStorageTester{NewAdminStorage: func() storage.AdminStorage {
		cleanTestDB(db, t)
		return NewAdminStorage(db)
	}}
	tester.RunAllTests(t)
}

func TestAdminTX_CreateTree_InitializesStorageStructures(t *testing.T) {
	cleanTestDB(db, t)
	s := NewAdminStorage(db)
	ctx := context.Background()

	tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() failed: %v", err)
	}

	// Check if TreeControl is correctly written.
	var signingEnabled, sequencingEnabled bool
	var sequenceIntervalSeconds int
	if err := db.QueryRowContext(ctx, selectTreeControlByID, tree.TreeId).Scan(&signingEnabled, &sequencingEnabled, &sequenceIntervalSeconds); err != nil {
		t.Fatalf("Failed to read TreeControl: %v", err)
	}
	// We don't mind about specific values, defaults change, but let's check
	// that important numbers are not zeroed.
	if sequenceIntervalSeconds <= 0 {
		t.Errorf("sequenceIntervalSeconds = %v, want > 0", sequenceIntervalSeconds)
	}
}

func TestCreateTreeInvalidStates(t *testing.T) {
	cleanTestDB(db, t)
	s := NewAdminStorage(db)
	ctx := context.Background()

	states := []trillian.TreeState{trillian.TreeState_DRAINING, trillian.TreeState_FROZEN}

	for _, state := range states {
		inTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
		inTree.TreeState = state
		if _, err := storage.CreateTree(ctx, s, inTree); err == nil {
			t.Errorf("CreateTree() state: %v got: nil want: err", state)
		}
	}
}

func TestAdminTX_TreeWithNulls(t *testing.T) {
	cleanTestDB(db, t)
	s := NewAdminStorage(db)
	ctx := context.Background()

	// Setup: create a tree and set all nullable columns to null.
	// Some columns have to be manually updated, as it's not possible to set
	// some proto fields to nil.
	tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() failed: %v", err)
	}
	treeID := tree.TreeId

	if err := setNulls(ctx, db, treeID); err != nil {
		t.Fatalf("setNulls() = %v, want = nil", err)
	}

	tests := []struct {
		desc string
		fn   storage.AdminTXFunc
	}{
		{
			desc: "GetTree",
			fn: func(ctx context.Context, tx storage.AdminTX) error {
				_, err := tx.GetTree(ctx, treeID)
				return err
			},
		},
		{
			// ListTreeIDs *shouldn't* care about other columns, but let's test it just
			// in case.
			desc: "ListTreeIDs",
			fn: func(ctx context.Context, tx storage.AdminTX) error {
				ids, err := tx.ListTreeIDs(ctx, false /* includeDeleted */)
				if err != nil {
					return err
				}
				for _, id := range ids {
					if id == treeID {
						return nil
					}
				}
				return fmt.Errorf("ID not found: %v", treeID)
			},
		},
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, tx storage.AdminTX) error {
				trees, err := tx.ListTrees(ctx, false /* includeDeleted */)
				if err != nil {
					return err
				}
				for _, tree := range trees {
					if tree.TreeId == treeID {
						return nil
					}
				}
				return fmt.Errorf("ID not found: %v", treeID)
			},
		},
	}
	for _, test := range tests {
		if err := s.ReadWriteTransaction(ctx, test.fn); err != nil {
			t.Errorf("%v: err = %v, want = nil", test.desc, err)
		}
	}
}

func TestAdminTX_StorageSettingsNotSupported(t *testing.T) {
	cleanTestDB(db, t)
	s := NewAdminStorage(db)
	ctx := context.Background()

	settings, err := ptypes.MarshalAny(&empty.Empty{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}

	tests := []struct {
		desc string
		// fn attempts to either create or update a tree with a non-nil, valid Any proto
		// on Tree.StorageSettings. It's expected to return an error.
		fn func(storage.AdminStorage) error
	}{
		{
			desc: "CreateTree",
			fn: func(s storage.AdminStorage) error {
				tree := proto.Clone(testonly.LogTree).(*trillian.Tree)
				tree.StorageSettings = settings
				_, err := storage.CreateTree(ctx, s, tree)
				return err
			},
		},
		{
			desc: "UpdateTree",
			fn: func(s storage.AdminStorage) error {
				tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
				if err != nil {
					t.Fatalf("CreateTree() failed with err = %v", err)
				}
				_, err = storage.UpdateTree(ctx, s, tree.TreeId, func(tree *trillian.Tree) { tree.StorageSettings = settings })
				return err
			},
		},
	}
	for _, test := range tests {
		if err := test.fn(s); err == nil {
			t.Errorf("%v: err = nil, want non-nil", test.desc)
		}
	}
}

func cleanTestDB(db *sql.DB, t *testing.T) {
	t.Helper()
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatal(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}

func setNulls(ctx context.Context, db *sql.DB, treeID int64) error {
	stmt, err := db.PrepareContext(ctx, `
	UPDATE trees SET
		display_name = NULL,
		description = NULL,
		delete_time_millis = NULL
	WHERE tree_id = $1`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, treeID)
	return err
}

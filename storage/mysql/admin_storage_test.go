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

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
)

const selectTreeControlByID = "SELECT SigningEnabled, SequencingEnabled, SequenceIntervalSeconds FROM TreeControl WHERE TreeId = ?"

func TestMysqlAdminStorage(t *testing.T) {
	tester := &testonly.AdminStorageTester{NewAdminStorage: func() storage.AdminStorage {
		cleanTestDB(DB)
		return NewAdminStorage(DB)
	}}
	tester.RunAllTests(t)
}

func TestAdminTX_CreateTree_InitializesStorageStructures(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() failed: %v", err)
	}

	// Check if TreeControl is correctly written.
	var signingEnabled, sequencingEnabled bool
	var sequenceIntervalSeconds int
	if err := DB.QueryRowContext(ctx, selectTreeControlByID, tree.TreeId).Scan(&signingEnabled, &sequencingEnabled, &sequenceIntervalSeconds); err != nil {
		t.Fatalf("Failed to read TreeControl: %v", err)
	}
	// We don't mind about specific values, defaults change, but let's check
	// that important numbers are not zeroed.
	if sequenceIntervalSeconds <= 0 {
		t.Errorf("sequenceIntervalSeconds = %v, want > 0", sequenceIntervalSeconds)
	}
}

func TestCreateTreeInvalidStates(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
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
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	// Setup: create a tree and set all nullable columns to null.
	// Some columns have to be manually updated, as it's not possible to set
	// some proto fields to nil.
	tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() failed: %v", err)
	}
	treeID := tree.TreeId

	if err := setNulls(ctx, DB, treeID); err != nil {
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
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	settings, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{})
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

func TestAdminTX_HardDeleteTree(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() returned err = %v", err)
	}

	if err := s.ReadWriteTransaction(ctx, func(ctx context.Context, tx storage.AdminTX) error {
		if _, err := tx.SoftDeleteTree(ctx, tree.TreeId); err != nil {
			return err
		}
		return tx.HardDeleteTree(ctx, tree.TreeId)
	}); err != nil {
		t.Fatalf("ReadWriteTransaction() returned err = %v", err)
	}

	// Unlike the HardDelete tests on AdminStorageTester, here we have the chance to poke inside the
	// database and check that the rows are gone, so let's do just that.
	// If there's no record on Trees, then there can be no record in any of the dependent tables.
	var name string
	if err := DB.QueryRowContext(ctx, "SELECT DisplayName FROM Trees WHERE TreeId = ?", tree.TreeId).Scan(&name); err != sql.ErrNoRows {
		t.Errorf("QueryRowContext() returned err = %v, want = %v", err, sql.ErrNoRows)
	}
}

func TestCheckDatabaseAccessible_Fails(t *testing.T) {
	// Pass in a closed database to provoke a failure.
	db := openTestDBOrDie()
	cleanTestDB(db)
	s := NewAdminStorage(db)
	db.Close()
	ctx := context.Background()
	if err := s.CheckDatabaseAccessible(ctx); err == nil {
		t.Error("TestCheckDatabaseAccessible_Fails got: nil, want: err")
	}
}

func TestCheckDatabaseAccessible_OK(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("TestCheckDatabaseAccessible_OK got: %v, want: nil", err)
	}
}

func setNulls(ctx context.Context, db *sql.DB, treeID int64) error {
	stmt, err := db.PrepareContext(ctx, "UPDATE Trees SET DisplayName = NULL, Description = NULL WHERE TreeId = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, treeID)
	return err
}

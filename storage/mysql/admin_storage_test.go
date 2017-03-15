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

	"github.com/google/trillian"
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

	tree, err := createTreeInternal(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("createTree() failed: %v", err)
	}

	// Check if TreeControl is correctly written.
	var signingEnabled, sequencingEnabled bool
	var sequenceIntervalSeconds int
	if err := DB.QueryRow(selectTreeControlByID, tree.TreeId).Scan(&signingEnabled, &sequencingEnabled, &sequenceIntervalSeconds); err != nil {
		t.Fatalf("Failed to read TreeControl: %v", err)
	}
	// We don't mind about specific values, defaults change, but let's check
	// that important numbers are not zeroed.
	if sequenceIntervalSeconds <= 0 {
		t.Errorf("sequenceIntervalSeconds = %v, want > 0", sequenceIntervalSeconds)
	}
}

func TestAdminTX_TreeWithNulls(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	// Setup: create a tree and set all nullable columns to null.
	// Some columns have to be manually updated, as it's not possible to set
	// some proto fields to nil.
	tree, err := createTreeInternal(ctx, s, testonly.LogTree)
	if err != nil {
		t.Fatalf("createTree() failed: %v", err)
	}
	if err := setNulls(DB, tree.TreeId); err != nil {
		t.Fatalf("setNulls() = %v, want = nil", err)
	}

	tests := []struct {
		desc string
		fn   func(context.Context, storage.AdminTX, int64) error
	}{
		{
			desc: "GetTree",
			fn: func(ctx context.Context, tx storage.AdminTX, treeID int64) error {
				_, err := tx.GetTree(ctx, treeID)
				return err
			},
		},
		{
			// ListTreeIDs *shouldn't* care about other columns, but let's test it just
			// in case.
			desc: "ListTreeIDs",
			fn: func(ctx context.Context, tx storage.AdminTX, treeID int64) error {
				ids, err := tx.ListTreeIDs(ctx)
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
			fn: func(ctx context.Context, tx storage.AdminTX, treeID int64) error {
				trees, err := tx.ListTrees(ctx)
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
		tx, err := s.Begin(ctx)
		if err != nil {
			t.Errorf("%v: Begin() = (_, %v), want = (_, nil)", test.desc, err)
			continue
		}
		defer tx.Close()
		if err := test.fn(ctx, tx, tree.TreeId); err != nil {
			t.Errorf("%v: err = %v, want = nil", test.desc, err)
			continue
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("%v: Commit() = %v, want = nil", test.desc, err)
		}
	}
}

func createTreeInternal(ctx context.Context, s storage.AdminStorage, tree *trillian.Tree) (*trillian.Tree, error) {
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

func setNulls(db *sql.DB, treeID int64) error {
	stmt, err := db.Prepare("UPDATE Trees SET DisplayName = NULL, Description = NULL WHERE TreeId = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(treeID)
	return err
}

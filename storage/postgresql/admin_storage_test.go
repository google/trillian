// Copyright 2017 Google LLC. All Rights Reserved.
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

package postgresql

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/postgresql/postgresqlpb"
	"github.com/google/trillian/storage/testonly"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const selectTreeControlByID = "SELECT SigningEnabled, SequencingEnabled, SequenceIntervalSeconds FROM TreeControl WHERE TreeId = $1"

func TestPostgresqlAdminStorage(t *testing.T) {
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
	if err := DB.QueryRow(ctx, selectTreeControlByID, tree.TreeId).Scan(&signingEnabled, &sequencingEnabled, &sequenceIntervalSeconds); err != nil {
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

func TestAdminTX_StorageSettings(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	badSettings, err := anypb.New(&trillian.Tree{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}
	goodSettings, err := anypb.New(&postgresqlpb.StorageOptions{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}

	tests := []struct {
		desc string
		// fn attempts to either create or update a tree with a non-nil, valid Any proto
		// on Tree.StorageSettings. It's expected to return an error.
		fn      func(storage.AdminStorage) error
		wantErr bool
	}{
		{
			desc: "CreateTree Bad Settings",
			fn: func(s storage.AdminStorage) error {
				tree := proto.Clone(testonly.LogTree).(*trillian.Tree)
				tree.StorageSettings = badSettings
				_, err := storage.CreateTree(ctx, s, tree)
				return err
			},
			wantErr: true,
		},
		{
			desc: "CreateTree nil Settings",
			fn: func(s storage.AdminStorage) error {
				tree := proto.Clone(testonly.LogTree).(*trillian.Tree)
				tree.StorageSettings = nil
				_, err := storage.CreateTree(ctx, s, tree)
				return err
			},
			wantErr: false,
		},
		{
			desc: "CreateTree StorageOptions Settings",
			fn: func(s storage.AdminStorage) error {
				tree := proto.Clone(testonly.LogTree).(*trillian.Tree)
				tree.StorageSettings = goodSettings
				_, err := storage.CreateTree(ctx, s, tree)
				return err
			},
			wantErr: false,
		},
		{
			desc: "UpdateTree",
			fn: func(s storage.AdminStorage) error {
				tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
				if err != nil {
					t.Fatalf("CreateTree() failed with err = %v", err)
				}
				_, err = storage.UpdateTree(ctx, s, tree.TreeId, func(tree *trillian.Tree) { tree.StorageSettings = badSettings })
				return err
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		if err := test.fn(s); (err != nil) != test.wantErr {
			t.Errorf("err: %v, wantErr = %v", err, test.wantErr)
		}
	}
}

// Test reading variants of trees that could have been created by old versions
// of Trillian to check we infer the correct storage options.
func TestAdminTX_GetTreeLegacies(t *testing.T) {
	cleanTestDB(DB)
	s := NewAdminStorage(DB)
	ctx := context.Background()

	serializedStorageSettings := func(revisioned bool) []byte {
		ss := storageSettings{
			Revisioned: revisioned,
		}
		buff := &bytes.Buffer{}
		enc := gob.NewEncoder(buff)
		if err := enc.Encode(ss); err != nil {
			t.Fatalf("failed to encode storageSettings: %v", err)
		}
		return buff.Bytes()
	}
	tests := []struct {
		desc           string
		key            []byte
		wantRevisioned bool
	}{
		{
			desc:           "No data",
			key:            []byte{},
			wantRevisioned: true,
		},
		{
			desc:           "Public key",
			key:            []byte("trustmethatthisisapublickey"),
			wantRevisioned: true,
		},
		{
			desc:           "StorageOptions revisioned",
			key:            serializedStorageSettings(true),
			wantRevisioned: true,
		},
		{
			desc:           "StorageOptions revisionless",
			key:            serializedStorageSettings(false),
			wantRevisioned: false,
		},
	}
	for _, tC := range tests {
		// Create a tree with default settings, and then reach into the DB to override
		// whatever was written into the persisted settings to align with the test case.
		tree, err := storage.CreateTree(ctx, s, testonly.LogTree)
		if err != nil {
			t.Fatal(err)
		}
		// We are reaching really into the internals here, but it's the only way to set up
		// archival state. Going through the Create/Update methods will change the storage
		// options.
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("UPDATE Trees SET PublicKey = $1 WHERE TreeId = $2", tC.key, tree.TreeId); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		readTree, err := storage.GetTree(ctx, s, tree.TreeId)
		if err != nil {
			t.Fatal(err)
		}
		o := &postgresqlpb.StorageOptions{}
		if err := anypb.UnmarshalTo(readTree.StorageSettings, o, proto.UnmarshalOptions{}); err != nil {
			t.Fatal(err)
		}
		if got, want := o.SubtreeRevisions, tC.wantRevisioned; got != want {
			t.Errorf("%s SubtreeRevisions: got %t, wanted %t", tC.desc, got, want)
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
	if err := DB.QueryRow(ctx, "SELECT DisplayName FROM Trees WHERE TreeId = $1", tree.TreeId).Scan(&name); err != pgx.ErrNoRows {
		t.Errorf("QueryRow() returned err = %v, want = %v", err, pgx.ErrNoRows)
	}
}

func TestCheckDatabaseAccessible_Fails(t *testing.T) {
	ctx := context.Background()

	// Pass in a closed database to provoke a failure.
	db, done := openTestDBOrDie()
	cleanTestDB(db)
	s := NewAdminStorage(db)
	done(ctx)

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

func setNulls(ctx context.Context, db *pgxpool.Pool, treeID int64) error {
	_, err := db.Exec(ctx, "UPDATE Trees SET DisplayName = NULL, Description = NULL WHERE TreeId = $1", treeID)
	return err
}

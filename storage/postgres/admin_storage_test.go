// Copyright 2018 Google Inc. All Rights Reserved.
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

package postgres_test

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/postgres"
	"github.com/google/trillian/storage/postgres/testdb"
	"github.com/google/trillian/storage/testonly"
)

var allTables = []string{"unsequenced", "tree_head", "sequenced_leaf_data", "leaf_data", "subtree", "tree_control", "trees"}
var DB *sql.DB

const selectTreeControlByID = "SELECT signing_enabled, sequencing_enabled, sequence_interval_seconds FROM tree_control WHERE tree_id = $1"

func TestPgAdminStorage(t *testing.T) {
	tester := &testonly.AdminStorageTester{NewAdminStorage: func() storage.AdminStorage {
		cleanTestDB(DB, t)
		return postgres.NewAdminStorage(DB)
	}}
	tester.TestCreateTree(t)
	tester.TestAdminTXReadWriteTransaction(t)
}

func TestAdminTX_CreateTree_InitializesStorageStructures(t *testing.T) {
	cleanTestDB(DB, t)
	s := postgres.NewAdminStorage(DB)
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
	cleanTestDB(DB, t)
	s := postgres.NewAdminStorage(DB)
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

func cleanTestDB(db *sql.DB, t *testing.T) {
	t.Helper()
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatal(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}

func openTestDBOrDie() *sql.DB {
	db, err := testdb.NewTrillianDB(context.TODO())
	if err != nil {
		panic(err)
	}
	return db
}
func TestMain(m *testing.M) {
	flag.Parse()
	if !testdb.PGAvailable() {
		glog.Errorf("PG not available, skipping all PG storage tests")
		return
	}
	DB = openTestDBOrDie()
	defer DB.Close()
	ec := m.Run()
	os.Exit(ec)
}

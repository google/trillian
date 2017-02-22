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
	"testing"

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
	tx, err := s.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()
	tree, err := tx.CreateTree(ctx, testonly.LogTree)
	if err != nil {
		t.Fatalf("CreateTree() = (_, %v), want = (_, nil)", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() = %v, want = nil", err)
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

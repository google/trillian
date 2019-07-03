// Copyright 2019 Google Inc. All Rights Reserved.
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
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"

	storageto "github.com/google/trillian/storage/testonly"
)

// MapTests is a suite of tests to run against the storage.MapTest interface.
type MapTests struct{}

func (*MapTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.MapStorage, _ storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func (*MapTests) TestMapSnapshot(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage) {
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
			if err != nil {
				t.Fatalf("SnapshotForTree()=_,%v; want _, nil", err)
			}
			defer tx.Close()

			_, err = tx.LatestSignedMapRoot(ctx)
			if gotErr := (err != nil); gotErr != test.wantErr {
				t.Errorf("LatestSignedMapRoot()=_,%v; want _, err? %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit()=_,%v; want _,nil", err)
			}
		})
	}
}

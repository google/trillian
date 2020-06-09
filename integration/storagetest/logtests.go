// Copyright 2020 Google Inc. All Rights Reserved.
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

package storagetest

import (
	"context"
	"crypto"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
)

// LogStorageFactory creates LogStorage and AdminStorage for a test to use.
type LogStorageFactory = func(ctx context.Context, t *testing.T) (storage.LogStorage, storage.AdminStorage)

// LogStorageTest executes a test using the given storage implementations.
type LogStorageTest = func(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage)

// RunLogStorageTests runs all the log storage tests against the provided log storage implementation.
func RunLogStorageTests(t *testing.T, storageFactory LogStorageFactory) {
	ctx := context.Background()
	for name, f := range logTestFunctions(t, &LogTests{}) {
		s, as := storageFactory(ctx, t)
		t.Run(name, func(t *testing.T) { f(ctx, t, s, as) })
	}
}

func logTestFunctions(t *testing.T, x interface{}) map[string]LogStorageTest {
	tests := make(map[string]LogStorageTest)
	xv := reflect.ValueOf(x)
	for _, name := range testFunctions(x) {
		m := xv.MethodByName(name)
		if !m.IsValid() {
			t.Fatalf("storagetest: function %v is not valid", name)
		}
		i := m.Interface()
		f, ok := i.(LogStorageTest)
		if !ok {
			// Method exists but has the wrong type signature.
			t.Fatalf("storagetest: function %v has unexpected signature %T, %v", name, m.Interface(), m)
		}
		nickname := strings.TrimPrefix(name, "Test")
		tests[nickname] = f
	}
	return tests
}

// LogTests is a suite of tests to run against the storage.LogTest interface.
type LogTests struct{}

func (*LogTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func (*LogTests) TestSnapshot(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	frozenLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, frozenLog, 0)
	if _, err := storage.UpdateTree(ctx, as, frozenLog.TreeId, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	}); err != nil {
		t.Fatalf("Error updating frozen tree: %v", err)
	}

	activeLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, activeLog, 0)
	mapTreeID := mustCreateTree(ctx, t, as, storageto.MapTree).TreeId

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "unknownSnapshot",
			tree:    logTree(-1),
			wantErr: true,
		},
		{
			desc: "activeLogSnapshot",
			tree: activeLog,
		},
		{
			desc: "frozenSnapshot",
			tree: frozenLog,
		},
		{
			desc:    "mapSnapshot",
			tree:    logTree(mapTreeID),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tx, err := s.SnapshotForTree(ctx, test.tree)

			if err == storage.ErrTreeNeedsInit {
				defer tx.Close()
			}

			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("err = %q, wantErr = %v", err, test.wantErr)
			} else if hasErr {
				return
			}
			defer tx.Close()

			_, err = tx.LatestSignedLogRoot(ctx)
			if err != nil {
				t.Errorf("LatestSignedLogRoot() returned err = %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit() returned err = %v", err)
			}
		})
	}
}

func logTree(logID int64) *trillian.Tree {
	return &trillian.Tree{
		TreeId:       logID,
		TreeType:     trillian.TreeType_LOG,
		HashStrategy: trillian.HashStrategy_RFC6962_SHA256,
	}
}

func mustSignAndStoreLogRoot(ctx context.Context, t *testing.T, l storage.LogStorage, tree *trillian.Tree, treeSize uint64) {
	t.Helper()
	signer := tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notnil")), crypto.SHA256)

	err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		root, err := signer.SignLogRoot(&types.LogRootV1{TreeSize: treeSize, RootHash: []byte{0}})
		if err != nil {
			return fmt.Errorf("error creating new SignedLogRoot: %v", err)
		}
		if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
			return fmt.Errorf("error storing new SignedLogRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}
}

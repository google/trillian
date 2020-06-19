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
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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
	for name, f := range logTestFunctions(t, &logTests{}) {
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

// logTests is a suite of tests to run against the storage.LogTest interface.
type logTests struct{}

func (*logTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func (*logTests) TestSnapshot(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
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
				t.Fatalf("err: %v, wantErr = %v", err, test.wantErr)
			} else if hasErr {
				return
			}
			defer tx.Close()

			_, err = tx.LatestSignedLogRoot(ctx)
			if err != nil {
				t.Errorf("LatestSignedLogRoot() returned err: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit() returned err: %v", err)
			}
		})
	}
}

func (*logTests) TestReadWriteTransaction(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	activeLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, activeLog, 0)

	tests := []struct {
		desc          string
		tree          *trillian.Tree
		wantNeedsInit bool
		wantErr       bool
		wantLogRoot   []byte
		wantTXRev     int64
	}{
		{
			desc:          "uninitializedBegin",
			tree:          logTree(-1),
			wantNeedsInit: true,
			wantTXRev:     0,
		},
		{
			desc: "activeLogBegin",
			tree: activeLog,
			wantLogRoot: func() []byte {
				b, err := (&types.LogRootV1{RootHash: []byte{0}}).MarshalBinary()
				if err != nil {
					panic(err)
				}
				return b
			}(),
			wantTXRev: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			err := s.ReadWriteTransaction(ctx, test.tree, func(ctx context.Context, tx storage.LogTreeTX) error {
				root, err := tx.LatestSignedLogRoot(ctx)
				if err != nil && !(err == storage.ErrTreeNeedsInit && test.wantNeedsInit) {
					t.Fatalf("%v: LatestSignedLogRoot() returned err = %v", test.desc, err)
				}
				gotRev, _ := tx.WriteRevision(ctx)
				if gotRev != test.wantTXRev {
					t.Errorf("%v: WriteRevision() = %v, want = %v", test.desc, gotRev, test.wantTXRev)
				}
				if got, want := root.GetLogRoot(), test.wantLogRoot; !bytes.Equal(got, want) {
					var logRoot types.LogRootV1
					if err := logRoot.UnmarshalBinary(got); err != nil {
						t.Error(err)
					}
					t.Errorf("%v: LogRoot: \n%x, want \n%x \nUnpacked: %#v", test.desc, got, want, logRoot)
				}
				return nil
			})
			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("%v: err = %q, wantErr = %v", test.desc, err, test.wantErr)
			} else if hasErr {
				return
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

// AddSequencedLeaves tests. ---------------------------------------------------

type addSequencedLeavesTest struct {
	t    *testing.T
	s    storage.LogStorage
	tree *trillian.Tree
}

func initAddSequencedLeavesTest(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) addSequencedLeavesTest {
	tree := mustCreateTree(ctx, t, as, storageto.PreorderedLogTree)
	mustSignAndStoreLogRoot(ctx, t, s, tree, 0)
	return addSequencedLeavesTest{t, s, tree}
}

func (t *addSequencedLeavesTest) addSequencedLeaves(leaves []*trillian.LogLeaf) {
	ctx := context.TODO()
	// Time we will queue all leaves at.
	var fakeQueueTime = time.Date(2016, 11, 10, 15, 16, 27, 0, time.UTC)

	queued, err := t.s.AddSequencedLeaves(ctx, t.tree, leaves, fakeQueueTime)
	if err != nil {
		t.t.Fatalf("Failed to add sequenced leaves: %v", err)
	}
	if got, want := len(queued), len(leaves); got != want {
		t.t.Errorf("AddSequencedLeaves(): %v queued leaves, want %v", got, want)
	}
	// TODO(pavelkalinnikov): Verify returned status for each leaf.
}

func (t *addSequencedLeavesTest) verifySequencedLeaves(start, count int64, exp []*trillian.LogLeaf) {
	var stored []*trillian.LogLeaf
	runLogTX(t.s, t.tree, t.t, func(ctx context.Context, tx storage.LogTreeTX) error {
		var err error
		t.t.Logf("GetLeavesByRange(%v, %v)", start, count)
		stored, err = tx.GetLeavesByRange(ctx, start, count)
		if err != nil {
			t.t.Fatalf("Failed to read sequenced leaves: %v", err)
		}
		return nil
	})
	if got, want := len(stored), len(exp); got != want {
		t.t.Fatalf("Unexpected number of leaves: got %d, want %d", got, want)
	}

	for i, leaf := range stored {
		if got, want := leaf.LeafIndex, exp[i].LeafIndex; got != want {
			t.t.Fatalf("Leaf #%d: LeafIndex=%v, want %v", i, got, want)
		}
		if got, want := leaf.LeafIdentityHash, exp[i].LeafIdentityHash; !bytes.Equal(got, want) {
			t.t.Fatalf("Leaf #%d: LeafIdentityHash=%v, want %v", i, got, want)
		}
	}
}

func (*logTests) TestAddSequencedLeavesUnordered(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	const chunk = 5
	const count = chunk * 5
	const extraCount = 16
	leaves := createTestLeaves(count, 0)

	aslt := initAddSequencedLeavesTest(ctx, t, s, as)
	for _, idx := range []int{1, 0, 4, 2} {
		aslt.addSequencedLeaves(leaves[chunk*idx : chunk*(idx+1)])
	}
	aslt.verifySequencedLeaves(0, count+extraCount, leaves[:chunk*3])
	aslt.verifySequencedLeaves(chunk*4, chunk+extraCount, leaves[chunk*4:count])
	aslt.addSequencedLeaves(leaves[chunk*3 : chunk*4])
	aslt.verifySequencedLeaves(0, count+extraCount, leaves)
}

func (*logTests) TestAddSequencedLeavesWithDuplicates(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	leaves := createTestLeaves(6, 0)

	aslt := initAddSequencedLeavesTest(ctx, t, s, as)
	aslt.addSequencedLeaves(leaves[:3])
	aslt.verifySequencedLeaves(0, 3, leaves[:3])
	aslt.addSequencedLeaves(leaves[2:]) // Full dup.
	aslt.verifySequencedLeaves(0, 6, leaves)

	dupLeaves := createTestLeaves(4, 6)
	dupLeaves[0].LeafIdentityHash = leaves[0].LeafIdentityHash // Hash dup.
	dupLeaves[2].LeafIndex = 2                                 // Index dup.
	leafHash := sha256.Sum256([]byte("foobar"))
	dupLeaves[2].LeafIdentityHash = leafHash[:] // TODO: Remove when spannertest has transaction support.
	aslt.addSequencedLeaves(dupLeaves)
	aslt.verifySequencedLeaves(6, 4, nil)
	aslt.verifySequencedLeaves(7, 4, dupLeaves[1:2])
	aslt.verifySequencedLeaves(8, 4, nil)
	aslt.verifySequencedLeaves(9, 4, dupLeaves[3:4])

	dupLeaves = createTestLeaves(4, 6)
	aslt.addSequencedLeaves(dupLeaves)
	aslt.verifySequencedLeaves(6, 4, dupLeaves)
}

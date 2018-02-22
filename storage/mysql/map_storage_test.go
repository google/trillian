// Copyright 2016 Google Inc. All Rights Reserved.
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
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly"
	"github.com/kylelemons/godebug/pretty"

	spb "github.com/google/trillian/crypto/sigpb"
)

var readMapOpts = storage.NewGetOpts(storage.Admin, true, trillian.TreeType_MAP)
var writeMapOpts = storage.NewGetOpts(storage.Admin, false, trillian.TreeType_MAP)

func TestMySQLMapStorage_CheckDatabaseAccessible(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	s := NewMapStorage(DB)
	if err := s.CheckDatabaseAccessible(context.Background()); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func TestMapSnapshot(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()

	frozenMapID := createInitializedMapForTests(ctx, t, DB)
	updateTree(DB, frozenMapID, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	})

	activeMapID := createInitializedMapForTests(ctx, t, DB)
	logID := createLogForTests(DB)

	tests := []struct {
		desc    string
		mapID   int64
		wantErr bool
	}{
		{
			desc:    "unknownSnapshot",
			mapID:   -1,
			wantErr: true,
		},
		{
			desc:  "activeMapSnapshot",
			mapID: activeMapID,
		},
		{
			desc:  "frozenSnapshot",
			mapID: frozenMapID,
		},
		{
			desc:    "logSnapshot",
			mapID:   logID,
			wantErr: true,
		},
	}

	s := NewMapStorage(DB)
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tree := &trillian.Tree{TreeId: test.mapID, TreeType: trillian.TreeType_MAP}
			tx, err := s.SnapshotForTree(ctx, tree)

			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("%v: err = %q, wantErr = %v", test.desc, err, test.wantErr)
			} else if hasErr {
				return
			}
			defer tx.Close()

			_, err = tx.LatestSignedMapRoot(ctx)
			if err != nil {
				t.Errorf("%v: LatestSignedMapRoot() returned err = %v", test.desc, err)
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("%v: Commit() returned err = %v", test.desc, err)
			}
		})
	}
}

func TestMapReadWriteTransaction(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()

	frozenMapID := createInitializedMapForTests(ctx, t, DB)
	updateTree(DB, frozenMapID, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	})

	activeMapID := createInitializedMapForTests(ctx, t, DB)
	logID := createLogForTests(DB)

	tests := []struct {
		desc    string
		mapID   int64
		wantErr bool
	}{
		{
			desc:    "unknownBegin",
			mapID:   -1,
			wantErr: true,
		},
		{
			desc:  "activeMapBegin",
			mapID: activeMapID,
		},
		{
			desc:    "frozenBegin",
			mapID:   frozenMapID,
			wantErr: true,
		},
		{
			desc:    "logBegin",
			mapID:   logID,
			wantErr: true,
		},
	}

	s := NewMapStorage(DB)
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tree := &trillian.Tree{TreeId: test.mapID, TreeType: trillian.TreeType_MAP}
			err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
				root, err := tx.LatestSignedMapRoot(ctx)
				if err != nil {
					t.Errorf("%v: LatestSignedMapRoot() returned err = %v", test.desc, err)
				}
				if got, want := tx.WriteRevision(), root.MapRevision+1; got != want {
					t.Errorf("%v: WriteRevision() = %v, want = %v", test.desc, got, want)
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

func TestMapRootUpdate(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	populatedMetadata := testonly.MustMarshalAny(t, &ctmapperpb.MapperMetadata{HighestFullyCompletedSeq: 1})

	for _, tc := range []struct {
		desc         string
		root         trillian.SignedMapRoot
		wantMetadata *any.Any
	}{
		{
			desc: "Initial root",
			root: trillian.SignedMapRoot{
				MapId:          mapID,
				TimestampNanos: 98765,
				MapRevision:    5,
				RootHash:       []byte(dummyHash),
				Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
			},
		},
		{
			desc: "Root update",
			root: trillian.SignedMapRoot{
				MapId:          mapID,
				TimestampNanos: 98766,
				MapRevision:    6,
				RootHash:       []byte(dummyHash),
				Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
			},
		},
		{
			desc: "Root with default (empty) MapperMetadata",
			root: trillian.SignedMapRoot{
				MapId:          mapID,
				TimestampNanos: 98768,
				MapRevision:    7,
				RootHash:       []byte(dummyHash),
				Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
				Metadata:       &any.Any{},
			},
		},
		{
			desc: "Root with non-default (populated) MapperMetadata",
			root: trillian.SignedMapRoot{
				MapId:          mapID,
				TimestampNanos: 98769,
				MapRevision:    8,
				RootHash:       []byte(dummyHash),
				Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
				Metadata:       populatedMetadata,
			},
			wantMetadata: populatedMetadata,
		},
	} {
		func() {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				if err := tx.StoreSignedMapRoot(ctx, tc.root); err != nil {
					t.Fatalf("%v: Failed to store signed map root: %v", tc.desc, err)
				}
				return nil
			})
		}()

		func() {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				root, err := tx.LatestSignedMapRoot(ctx)
				if err != nil {
					t.Fatalf("%v: Failed to read back new map root: %v", tc.desc, err)
				}

				want := proto.Clone(&tc.root).(*trillian.SignedMapRoot)
				want.Metadata = tc.wantMetadata
				if got := &root; !proto.Equal(got, want) {
					t.Errorf("%v: LatestSignedMapRoot() diff(-got, +want) \n%v", tc.desc, pretty.Compare(got, want))
				}
				return nil
			})
		}()
	}
}

var keyHash = []byte([]byte("A Key Hash"))
var mapLeaf = trillian.MapLeaf{
	Index:     keyHash,
	LeafHash:  []byte("A Hash"),
	LeafValue: []byte("A Value"),
	ExtraData: []byte("Some Extra Data"),
}

func TestMapSetGetRoundTrip(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	readRev := int64(1)
	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err != nil {
				t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
			}
			return nil
		})
	}

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			readValues, err := tx.Get(ctx, readRev, [][]byte{keyHash})
			if err != nil {
				t.Fatalf("Failed to get %v:  %v", keyHash, err)
			}
			if got, want := len(readValues), 1; got != want {
				t.Fatalf("Got %d values, expected %d", got, want)
			}
			if got, want := &readValues[0], &mapLeaf; !proto.Equal(got, want) {
				t.Fatalf("Read back %v, but expected %v", got, want)
			}
			return nil
		})
	}
}

func TestMapSetSameKeyInSameRevisionFails(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err != nil {
				t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
			}
			return nil
		})
	}

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err == nil {
				t.Fatalf("Unexpectedly succeeded in setting %v to %v", keyHash, mapLeaf)
			}
			return nil
		})
	}
}

func TestMapGet0Results(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	for _, tc := range []struct {
		index [][]byte
	}{
		{index: nil}, //empty list.
		{index: [][]byte{[]byte("This doesn't exist.")}},
	} {
		t.Run(fmt.Sprintf("tx.Get(%s)", tc.index), func(t *testing.T) {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				readValues, err := tx.Get(ctx, 1, tc.index)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := len(readValues), 0; got != want {
					t.Fatalf("len = %d, want %d", got, want)
				}
				return nil
			})
		})
	}
}

func TestMapSetGetMultipleRevisions(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	tests := []struct {
		rev  int64
		leaf trillian.MapLeaf
	}{
		{0, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{0}, LeafValue: []byte{0}, ExtraData: []byte{0}}},
		{1, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{1}, LeafValue: []byte{1}, ExtraData: []byte{1}}},
		{2, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{2}, LeafValue: []byte{2}, ExtraData: []byte{2}}},
		{3, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{3}, LeafValue: []byte{3}, ExtraData: []byte{3}}},
	}

	for _, tc := range tests {
		func() {
			// Write the current test case.
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				mapTX := tx.(*mapTreeTX)
				mapTX.treeTX.writeRevision = tc.rev
				if err := tx.Set(ctx, keyHash, tc.leaf); err != nil {
					t.Fatalf("Failed to set %v to %v: %v", keyHash, tc.leaf, err)
				}
				return nil
			})

			// Read at a point in time in the future. Expect to get the latest value.
			// Read at each point in the past. Expect to get that exact point in history.
			for i := int64(0); i < int64(len(tests)); i++ {
				func() {
					expectRev := i
					if expectRev > tc.rev {
						expectRev = tc.rev // For future revisions, expect the current value.
					}

					runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
						readValues, err := tx2.Get(ctx, i, [][]byte{keyHash})
						if err != nil {
							t.Fatalf("At i %d failed to get %v:  %v", i, keyHash, err)
						}
						if got, want := len(readValues), 1; got != want {
							t.Fatalf("At i %d got %d values, expected %d", i, got, want)
						}
						if got, want := &readValues[0], &tests[expectRev].leaf; !proto.Equal(got, want) {
							t.Fatalf("At i %d read back %v, but expected %v", i, got, want)
						}
						return nil
					})
				}()
			}
		}()
	}
}

func TestGetSignedMapRootNotExist(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	mapID := createMapForTests(DB) // Uninitialized: no revision 0 MapRoot exists.
	s := NewMapStorage(DB)

	ctx := context.Background()
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}
	err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		_, err := tx.GetSignedMapRoot(ctx, 0)
		if got, want := err, storage.ErrTreeNeedsInit; got != want {
			t.Fatalf("GetSignedMapRoot: %v, want %v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction: %v", err)
	}
}

func TestLatestSignedMapRootNoneWritten(t *testing.T) {
	// TODO(phad): I'm considering removing this test, because for an Map that has been
	// initialized there should always be the revision 0 SMR written to the DB, and
	// without initialization the error path is identical to that tested in the func
	// TestGetSignedMapRootNotExist above.
	t.Skip("TODO: remove this as it can no longer occur.")

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		root, err := tx.LatestSignedMapRoot(ctx)
		if err != nil {
			t.Fatalf("Failed to read an empty map root: %v", err)
		}
		if root.MapId != 0 || len(root.RootHash) != 0 || root.Signature != nil {
			t.Fatalf("Read a root with contents when it should be empty: %v", root)
		}
		return nil
	})
}

func TestGetSignedMapRoot(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	revision := int64(5)
	root := trillian.SignedMapRoot{
		MapId:          mapID,
		TimestampNanos: 98765,
		MapRevision:    revision,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}
		return nil
	})

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
			root2, err := tx2.GetSignedMapRoot(ctx, revision)
			if err != nil {
				t.Fatalf("Failed to get back new map root: %v", err)
			}
			if !proto.Equal(&root, &root2) {
				t.Fatalf("Getting root round trip failed: <%#v> and: <%#v>", root, root2)
			}
			return nil
		})
	}
}

func TestLatestSignedMapRoot(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	root := trillian.SignedMapRoot{
		MapId:          mapID,
		TimestampNanos: 98765,
		MapRevision:    5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}
		return nil
	})

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
			root2, err := tx2.LatestSignedMapRoot(ctx)
			if err != nil {
				t.Fatalf("Failed to read back new map root: %v", err)
			}
			if !proto.Equal(&root, &root2) {
				t.Fatalf("Root round trip failed: <%#v> and: <%#v>", root, root2)
			}
			return nil
		})
	}
}

func TestDuplicateSignedMapRoot(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	ctx := context.Background()
	mapID := createInitializedMapForTests(ctx, t, DB)
	s := NewMapStorage(DB)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		root := trillian.SignedMapRoot{
			MapId:          mapID,
			TimestampNanos: 98765,
			MapRevision:    5,
			RootHash:       []byte(dummyHash),
			Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
		}
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed map root: %v", err)
		}
		// Shouldn't be able to do it again
		if err := tx.StoreSignedMapRoot(ctx, root); err == nil {
			t.Fatal("Allowed duplicate signed map root")
		}
		return nil
	})
}

func TestReadOnlyMapTX_Rollback(t *testing.T) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		t.Skipf("Inhibited due to known issue (#896) on SQL driver: %q", provider.Driver)
	}

	cleanTestDB(DB)
	s := NewMapStorage(DB)
	tx, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()
	// It's a bit hard to have a more meaningful test. This should suffice.
	if err := tx.Rollback(); err != nil {
		t.Errorf("Rollback() = (_, %v), want = (_, nil)", err)
	}
}

func runMapTX(ctx context.Context, s storage.MapStorage, tree *trillian.Tree, t *testing.T, f storage.MapTXFunc) {
	if err := s.ReadWriteTransaction(ctx, tree, f); err != nil {
		t.Fatalf("Failed to begin map tx: %v", err)
	}
}

func createInitializedMapForTests(ctx context.Context, t *testing.T, db *sql.DB) int64 {
	t.Helper()
	mapID := createMapForTests(db)

	s := NewMapStorage(db)
	tree := &trillian.Tree{TreeId: mapID, TreeType: trillian.TreeType_MAP}
	err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		initialRoot := trillian.SignedMapRoot{
			RootHash: []byte("rootHash"),
			Signature: &sigpb.DigitallySigned{
				Signature: []byte("sig"),
			},
			MapId:       mapID,
			MapRevision: 0,
		}

		if err := tx.StoreSignedMapRoot(ctx, initialRoot); err != nil {
			t.Fatalf("%v: Failed to StoreSignedMapRoot: %v", mapID, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}

	return mapID
}

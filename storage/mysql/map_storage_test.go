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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
)

func TestMySQLMapStorage_CheckDatabaseAccessible(t *testing.T) {
	cleanTestDB(DB)
	s, err := NewMapStorage(DB)
	if err != nil {
		t.Fatalf("NewMapStorage() = (_, %v), want = (_, nil)", err)
	}
	if err := s.CheckDatabaseAccessible(context.Background()); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func TestMapBegin(t *testing.T) {
	mapID := createMapID("TestBegin")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)

	storage, err := NewMapStorage(DB)
	if err != nil {
		t.Fatalf("NewMapStorage() = (_, %v), want = nil", err)
	}

	// TODO(codingllama): Add tree existence / type validation
	tests := []struct {
		mapID int64
	}{
		{mapID: mapID.mapID},
	}

	ctx := context.TODO()
	for _, test := range tests {
		tx, err := storage.BeginForTree(ctx, test.mapID)
		if err != nil {
			t.Fatalf("Begin() = (_, %v), want = (_, nil)", err)
		}

		root, err := tx.LatestSignedMapRoot()
		if err != nil {
			t.Errorf("LatestSignedMapRoot() = (_, %v), want = (_, nil)", err)
		}

		if got, want := tx.WriteRevision(), root.MapRevision+1; got != want {
			t.Errorf("WriteRevision() = %v, want = %v", got, want)
		}

		if err := tx.Commit(); err != nil {
			t.Errorf("Commit() = %v, want = nil", err)
		}
	}
}

func TestMapSnapshot(t *testing.T) {
	mapID := createMapID("TestBegin")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)

	storage, err := NewMapStorage(DB)
	if err != nil {
		t.Fatalf("NewMapStorage() = (_, %v), want = nil", err)
	}

	// TODO(codingllama): Add tree existence / type validation
	tests := []struct {
		mapID int64
	}{
		{mapID: mapID.mapID},
	}

	ctx := context.TODO()
	for _, test := range tests {
		tx, err := storage.SnapshotForTree(ctx, test.mapID)
		if err != nil {
			t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
		}

		// Do a read so we have something to commit on the snapshot
		_, err = tx.LatestSignedMapRoot()
		if err != nil {
			t.Errorf("LatestSignedMapRoot() = (_, %v), want = (_, nil)", err)
		}

		if err := tx.Commit(); err != nil {
			t.Errorf("Commit() = %v, want = nil", err)
		}
	}
}

func TestMapRootUpdate(t *testing.T) {
	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	mapID := createMapID("TestLatestSignedMapRoot")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()
	tx := beginMapTx(ctx, s, mapID, t)
	defer tx.Commit()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{
		MapId:          mapID.mapID,
		TimestampNanos: 98765,
		MapRevision:    5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root2 := trillian.SignedMapRoot{
		MapId:          mapID.mapID,
		TimestampNanos: 98766,
		MapRevision:    6,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}

	if err := tx.StoreSignedMapRoot(root2); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new map roots: %v", err)
	}

	tx = beginMapTx(ctx, s, mapID, t)
	root3, err := tx.LatestSignedMapRoot()

	if err != nil {
		t.Fatalf("Failed to read back new map root: %v", err)
	}

	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
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
	mapID := createMapID("TestMapSetGetRoundTrip")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)

	readRev := int64(1)

	ctx := context.Background()
	{
		tx := beginMapTx(ctx, s, mapID, t)

		if err := tx.Set(keyHash, mapLeaf); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	{
		tx := beginMapTx(ctx, s, mapID, t)

		readValues, err := tx.Get(readRev, [][]byte{keyHash})
		if err != nil {
			t.Fatalf("Failed to get %v:  %v", keyHash, err)
		}
		if got, want := len(readValues), 1; got != want {
			t.Fatalf("Got %d values, expected %d", got, want)
		}
		if got, want := &readValues[0], &mapLeaf; !proto.Equal(got, want) {
			t.Fatalf("Read back %v, but expected %v", got, want)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}
}

func TestMapSetSameKeyInSameRevisionFails(t *testing.T) {
	mapID := createMapID("TestMapSetSameKeyInSameRevisionFails")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()

	{
		tx := beginMapTx(ctx, s, mapID, t)

		if err := tx.Set(keyHash, mapLeaf); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	{
		tx := beginMapTx(ctx, s, mapID, t)

		if err := tx.Set(keyHash, mapLeaf); err == nil {
			t.Fatalf("Unexpectedly succeeded in setting %v to %v", keyHash, mapLeaf)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}
}

func TestMapGetUnknownKey(t *testing.T) {
	mapID := createMapID("TestMapGetUnknownKey")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()

	{
		tx := beginMapTx(ctx, s, mapID, t)

		readValues, err := tx.Get(1, [][]byte{[]byte("This doesn't exist.")})
		if err != nil {
			t.Fatalf("Read returned error %v", err)
		}
		if got, want := len(readValues), 0; got != want {
			t.Fatalf("Unexpectedly read %d values, expected %d", got, want)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}
}

func TestMapSetGetMultipleRevisions(t *testing.T) {
	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	mapID := createMapID("TestMapSetGetMultipleRevisions")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)

	tests := []struct {
		rev  int64
		leaf trillian.MapLeaf
	}{
		{0, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{0}, LeafValue: []byte{0}, ExtraData: []byte{0}}},
		{1, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{1}, LeafValue: []byte{1}, ExtraData: []byte{1}}},
		{2, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{2}, LeafValue: []byte{2}, ExtraData: []byte{2}}},
		{3, trillian.MapLeaf{Index: keyHash, LeafHash: []byte{3}, LeafValue: []byte{3}, ExtraData: []byte{3}}},
	}

	ctx := context.Background()
	for _, tc := range tests {
		// Write the current test case.
		tx := beginMapTx(ctx, s, mapID, t)
		mysqlMapTX := tx.(*mapTreeTX)
		mysqlMapTX.treeTX.writeRevision = tc.rev
		if err := tx.Set(keyHash, tc.leaf); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, tc.leaf, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Read at a point in time in the future. Expect to get the latest value.
		// Read at each point in the past. Expect to get that exact point in history.
		for i := int64(0); i < int64(len(tests)); i++ {
			expectRev := i
			if expectRev > tc.rev {
				expectRev = tc.rev // For future revisions, expect the current value.
			}
			tx2 := beginMapTx(ctx, s, mapID, t)
			readValues, err := tx2.Get(i, [][]byte{keyHash})
			if err != nil {
				t.Fatalf("At i %d failed to get %v:  %v", i, keyHash, err)
			}
			if got, want := len(readValues), 1; got != want {
				t.Fatalf("At i %d got %d values, expected %d", i, got, want)
			}
			if got, want := &readValues[0], &tests[expectRev].leaf; !proto.Equal(got, want) {
				t.Fatalf("At i %d read back %v, but expected %v", i, got, want)
			}
			if err := tx2.Commit(); err != nil {
				t.Fatalf("At i %d failed to commit: %v", i, err)
			}
		}
	}
}

func TestLatestSignedMapRootNoneWritten(t *testing.T) {
	mapID := createMapID("TestLatestSignedMapRootNoneWritten")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()
	tx := beginMapTx(ctx, s, mapID, t)
	defer tx.Rollback()

	root, err := tx.LatestSignedMapRoot()

	if err != nil {
		t.Fatalf("Failed to read an empty map root: %v", err)
	}

	if root.MapId != 0 || len(root.RootHash) != 0 || root.Signature != nil {
		t.Fatalf("Read a root with contents when it should be empty: %v", root)
	}
}

func TestLatestSignedMapRoot(t *testing.T) {
	mapID := createMapID("TestLatestSignedMapRoot")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()
	tx := beginMapTx(ctx, s, mapID, t)
	defer tx.Rollback()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &spb.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new map root: %v", err)
	}

	{
		tx2 := beginMapTx(ctx, s, mapID, t)
		defer tx2.Rollback()
		root2, err := tx2.LatestSignedMapRoot()

		if err != nil {
			t.Fatalf("Failed to read back new map root: %v", err)
		}

		if !proto.Equal(&root, &root2) {
			t.Fatalf("Root round trip failed: <%#v> and: <%#v>", root, root2)
		}
	}
}

func TestDuplicateSignedMapRoot(t *testing.T) {
	mapID := createMapID("TestDuplicateSignedMapRoot")
	cleanTestDB(DB)
	prepareTestMapDB(DB, mapID, t)
	s := prepareTestMapStorage(DB, t)
	ctx := context.Background()
	tx := beginMapTx(ctx, s, mapID, t)
	defer tx.Commit()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &spb.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	// Shouldn't be able to do it again
	if err := tx.StoreSignedMapRoot(root); err == nil {
		t.Fatal("Allowed duplicate signed map root")
	}
}

func TestReadOnlyMapTX_Rollback(t *testing.T) {
	cleanTestDB(DB)

	s, err := NewMapStorage(DB)
	if err != nil {
		t.Fatalf("NewMapStorage() = (_, %v), want = (_, nil)", err)
	}

	tx, err := s.Snapshot(context.TODO())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}

	// It's a bit hard to have a more meaningful test. This should suffice.
	if err := tx.Rollback(); err != nil {
		t.Errorf("Rollback() = (_, %v), want = (_, nil)", err)
	}
}

func prepareTestMapStorage(db *sql.DB, t *testing.T) storage.MapStorage {
	s, err := NewMapStorage(db)
	if err != nil {
		t.Fatalf("Failed to open map storage: %s", err)
	}
	return s
}

// This removes all database contents for the specified map id so tests run in a
// predictable environment. For obvious reasons this should only be allowed to run
// against test databases. This method panics if any of the deletions fails to make
// sure tests can't inadvertently succeed.
func prepareTestMapDB(db *sql.DB, mapID mapIDAndTest, t *testing.T) {
	prepareTestTreeDB(DB, mapID.mapID, t)
	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, mapID.mapID, mapID.mapID)
	if err != nil {
		t.Fatalf("Failed to create tree entry for test: %v", err)
	}
}

func beginMapTx(ctx context.Context, s storage.MapStorage, mapID mapIDAndTest, t *testing.T) storage.MapTreeTX {
	tx, err := s.BeginForTree(ctx, mapID.mapID)
	if err != nil {
		t.Fatalf("Failed to begin map tx: %v", err)
	}
	return tx
}

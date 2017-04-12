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
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"testing"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/testonly"
	"github.com/kylelemons/godebug/pretty"
)

var allTables = []string{"Unsequenced", "TreeHead", "SequencedLeafData", "LeafData", "Subtree", "TreeControl", "Trees", "MapLeaf", "MapHead"}

// Must be 32 bytes to match sha256 length if it was a real hash
var dummyHash = []byte("hashxxxxhashxxxxhashxxxxhashxxxx")
var dummyRawHash = []byte("xxxxhashxxxxhashxxxxhashxxxxhash")
var dummyHash2 = []byte("HASHxxxxhashxxxxhashxxxxhashxxxx")
var dummyHash3 = []byte("hashxxxxhashxxxxhashxxxxHASHxxxx")

// Time we will queue all leaves at
var fakeQueueTime = time.Date(2016, 11, 10, 15, 16, 27, 0, time.UTC)

// Time we'll request for guard cutoff in tests that don't test this (should include all above)
var fakeDequeueCutoffTime = time.Date(2016, 11, 10, 15, 16, 30, 0, time.UTC)

// Used for tests involving extra data
var someExtraData = []byte("Some extra data")

const leavesToInsert = 5
const sequenceNumber int64 = 237

// Tests that access the db should each use a distinct log ID to prevent lock contention when
// run in parallel or race conditions / unexpected interactions. Tests that pass should hold
// no locks afterwards.

func createFakeLeaf(ctx context.Context, db *sql.DB, logID int64, rawHash, hash, data, extraData []byte, seq int64, t *testing.T) *trillian.LogLeaf {
	_, err := db.ExecContext(ctx, "INSERT INTO LeafData(TreeId, LeafIdentityHash, LeafValue, ExtraData) VALUES(?,?,?,?)", logID, rawHash, data, extraData)
	_, err2 := db.ExecContext(ctx, "INSERT INTO SequencedLeafData(TreeId, SequenceNumber, LeafIdentityHash, MerkleLeafHash) VALUES(?,?,?,?)", logID, seq, rawHash, hash)

	if err != nil || err2 != nil {
		t.Fatalf("Failed to create test leaves: %v %v", err, err2)
	}
	return &trillian.LogLeaf{
		MerkleLeafHash:   hash,
		LeafValue:        data,
		ExtraData:        extraData,
		LeafIndex:        seq,
		LeafIdentityHash: rawHash,
	}
}

func TestMySQLLogStorage_CheckDatabaseAccessible(t *testing.T) {
	cleanTestDB(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)
	if err := s.CheckDatabaseAccessible(context.Background()); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func TestBeginSnapshot(t *testing.T) {
	cleanTestDB(dbWrapper)

	frozenLogID := testonly.CreateLogForTest(dbWrapper)
	if _, err := testonly.UpdateTreeForTest(dbWrapper, frozenLogID, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	}); err != nil {
		t.Fatalf("Error updating frozen tree: %v", err)
	}

	activeLogID := testonly.CreateLogForTest(dbWrapper)
	mapID := testonly.CreateMapForTest(dbWrapper)

	tests := []struct {
		desc  string
		logID int64
		// snapshot defines whether BeginForTree or SnapshotForTree is used for the test.
		snapshot, wantErr bool
	}{
		{
			desc:    "unknownBegin",
			logID:   -1,
			wantErr: true,
		},
		{
			desc:     "unknownSnapshot",
			logID:    -1,
			snapshot: true,
			wantErr:  true,
		},
		{
			desc:  "activeLogBegin",
			logID: activeLogID,
		},
		{
			desc:     "activeLogSnapshot",
			logID:    activeLogID,
			snapshot: true,
		},
		{
			desc:    "frozenBegin",
			logID:   frozenLogID,
			wantErr: true,
		},
		{
			desc:     "frozenSnapshot",
			logID:    frozenLogID,
			snapshot: true,
		},
		{
			desc:    "mapBegin",
			logID:   mapID,
			wantErr: true,
		},
		{
			desc:     "mapSnapshot",
			logID:    mapID,
			snapshot: true,
			wantErr:  true,
		},
	}

	ctx := context.Background()
	s := coresql.NewLogStorage(dbWrapper)
	for _, test := range tests {
		func() {
			var tx rootReaderLogTX
			var err error
			if test.snapshot {
				tx, err = s.SnapshotForTree(ctx, test.logID)
			} else {
				tx, err = s.BeginForTree(ctx, test.logID)
			}

			if hasErr := err != nil; hasErr != test.wantErr {
				t.Errorf("%v: err = %q, wantErr = %v", test.desc, err, test.wantErr)
				return
			} else if hasErr {
				return
			}
			defer tx.Close()

			root, err := tx.LatestSignedLogRoot(ctx)
			if err != nil {
				t.Errorf("%v: LatestSignedLogRoot() returned err = %v", test.desc, err)
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("%v: Commit() returned err = %v", test.desc, err)
			}

			if !test.snapshot {
				tx := tx.(storage.TreeTX)
				if got, want := tx.WriteRevision(), root.TreeRevision+1; got != want {
					t.Errorf("%v: WriteRevision() = %v, want = %v", test.desc, got, want)
				}
			}
		}()
	}
}

type rootReaderLogTX interface {
	storage.ReadOnlyTreeTX
	storage.LogRootReader
}

func TestIsOpenCommitRollbackClosed(t *testing.T) {
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tests := []struct {
		commit, rollback, close bool
	}{
		{commit: true},
		{rollback: true},
		{close: true},
	}
	for _, test := range tests {
		func() {
			tx := testonly.BeginLogTx(s, logID, t)
			defer tx.Close()
			if !tx.IsOpen() {
				t.Errorf("Transaction should be open on creation, test: %v", test)
			}
			var err error
			switch {
			case test.commit:
				err = tx.Commit()
			case test.rollback:
				err = tx.Rollback()
			case test.close:
				err = tx.Close()
			}
			if err != nil {
				t.Errorf("Failed to commit/rollback/close: %v, test = %v", err, test)
				return
			}
			if tx.IsOpen() {
				t.Errorf("Transaction should be closed after commit/rollback/close, test: %v", test)
			}
		}()
	}
}

func TestQueueDuplicateLeaf(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)
	count := 15
	leaves := createTestLeaves(int64(count), 10)
	leaves2 := createTestLeaves(int64(count), 12)
	leaves3 := createTestLeaves(3, 100)

	// Note that tests accumulate queued leaves on top of each other.
	var tests = []struct {
		leaves []*trillian.LogLeaf
		want   []*trillian.LogLeaf
	}{
		{
			// [10, 11, 12, ...]
			leaves: leaves,
			want:   make([]*trillian.LogLeaf, count),
		},
		{
			// [12, 13, 14, ...] so first (count-2) are duplicates
			leaves: leaves2,
			want:   append(leaves[2:], nil, nil),
		},
		{
			// [10, 100, 11, 101, 102] so [dup, new, dup, new, dup]
			leaves: []*trillian.LogLeaf{leaves[0], leaves3[0], leaves[1], leaves3[1], leaves[2]},
			want:   []*trillian.LogLeaf{leaves[0], nil, leaves[1], nil, leaves[2]},
		},
	}

	for _, test := range tests {
		func() {
			tx := testonly.BeginLogTx(s, logID, t)
			defer tx.Close()
			existing, err := tx.QueueLeaves(ctx, test.leaves, fakeQueueTime)
			if err != nil {
				t.Fatalf("Failed to queue leaves: %v", err)
			}
			testonly.Commit(tx, t)

			if len(existing) != len(test.want) {
				t.Errorf("|QueueLeaves()|=%d; want %d", len(existing), len(test.want))
				return
			}
			for i, want := range test.want {
				got := existing[i]
				if want == nil {
					if got != nil {
						t.Errorf("QueueLeaves()[%d]=%v; want nil", i, got)
					}
					continue
				}
				if got == nil {
					t.Errorf("QueueLeaves()[%d]=nil; want non-nil", i)
				} else if bytes.Compare(got.LeafIdentityHash, want.LeafIdentityHash) != 0 {
					t.Errorf("QueueLeaves()[%d].LeafIdentityHash=%x; want %x", i, got.LeafIdentityHash, want.LeafIdentityHash)
				}
			}
		}()
	}
}

func TestQueueLeaves(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	leaves := createTestLeaves(leavesToInsert, 20)
	if _, err := tx.QueueLeaves(ctx, leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}
	testonly.Commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the unsequenced data.
	var count int
	if err := dbWrapper.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID).Scan(&count); err != nil {
		t.Fatalf("Could not query row count: %v", err)
	}
	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}

	// Additional check on timestamp being set correctly in the database
	var queueTimestamp int64
	if err := dbWrapper.DB().QueryRowContext(ctx, "SELECT DISTINCT QueueTimestampNanos FROM Unsequenced WHERE TreeID=?", logID).Scan(&queueTimestamp); err != nil {
		t.Fatalf("Could not query timestamp: %v", err)
	}
	if got, want := queueTimestamp, fakeQueueTime.UnixNano(); got != want {
		t.Fatalf("Incorrect queue timestamp got: %d want: %d", got, want)
	}
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	leaves, err := tx.DequeueLeaves(ctx, 999, fakeDequeueCutoffTime)
	if err != nil {
		t.Fatalf("Didn't expect an error on dequeue with no work to be done: %v", err)
	}
	if len(leaves) > 0 {
		t.Fatalf("Expected nothing to be dequeued but we got %d leaves", len(leaves))
	}
	testonly.Commit(tx, t)
}

func TestDequeueLeaves(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if _, err := tx.QueueLeaves(ctx, leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		testonly.Commit(tx, t)
	}

	{
		// Now try to dequeue them
		tx2 := testonly.BeginLogTx(s, logID, t)
		defer tx2.Close()
		leaves2, err := tx2.DequeueLeaves(ctx, 99, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}
		if len(leaves2) != leavesToInsert {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}
		testonly.EnsureAllLeavesDistinct(leaves2, t)
		testonly.Commit(tx2, t)
	}

	{
		// If we dequeue again then we should now get nothing
		tx3 := testonly.BeginLogTx(s, logID, t)
		defer tx3.Close()
		leaves3, err := tx3.DequeueLeaves(ctx, 99, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}
		if len(leaves3) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves3))
		}
		testonly.Commit(tx3, t)
	}
}

func TestDequeueLeavesTwoBatches(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	leavesToDequeue1 := 3
	leavesToDequeue2 := 2

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if _, err := tx.QueueLeaves(ctx, leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		testonly.Commit(tx, t)
	}

	{
		// Now try to dequeue some of them
		tx2 := testonly.BeginLogTx(s, logID, t)
		defer tx2.Close()
		leaves2, err := tx2.DequeueLeaves(ctx, leavesToDequeue1, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}
		if len(leaves2) != leavesToDequeue1 {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}
		testonly.EnsureAllLeavesDistinct(leaves2, t)
		testonly.Commit(tx2, t)

		// Now try to dequeue the rest of them
		tx3 := testonly.BeginLogTx(s, logID, t)
		defer tx3.Close()
		leaves3, err := tx3.DequeueLeaves(ctx, leavesToDequeue2, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}
		if len(leaves3) != leavesToDequeue2 {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves3), leavesToDequeue2)
		}
		testonly.EnsureAllLeavesDistinct(leaves3, t)

		// Plus the union of the leaf batches should all have distinct hashes
		leaves4 := append(leaves2, leaves3...)
		testonly.EnsureAllLeavesDistinct(leaves4, t)
		testonly.Commit(tx3, t)
	}

	{
		// If we dequeue again then we should now get nothing
		tx4 := testonly.BeginLogTx(s, logID, t)
		defer tx4.Close()
		leaves5, err := tx4.DequeueLeaves(ctx, 99, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}
		if len(leaves5) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves5))
		}
		testonly.Commit(tx4, t)
	}
}

// Queues leaves and attempts to dequeue before the guard cutoff allows it. This should
// return nothing. Then retry with an inclusive guard cutoff and ensure the leaves
// are returned.
func TestDequeueLeavesGuardInterval(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if _, err := tx.QueueLeaves(ctx, leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		testonly.Commit(tx, t)
	}

	{
		// Now try to dequeue them using a cutoff that means we should get none
		tx2 := testonly.BeginLogTx(s, logID, t)
		defer tx2.Close()
		leaves2, err := tx2.DequeueLeaves(ctx, 99, fakeQueueTime.Add(-time.Second))
		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}
		if len(leaves2) != 0 {
			t.Fatalf("Dequeued %d leaves when they all should be in guard interval", len(leaves2))
		}

		// Try to dequeue again using a cutoff that should include them
		leaves2, err = tx2.DequeueLeaves(ctx, 99, fakeQueueTime.Add(time.Second))
		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}
		if len(leaves2) != leavesToInsert {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}
		testonly.EnsureAllLeavesDistinct(leaves2, t)
		testonly.Commit(tx2, t)
	}
}

func TestDequeueLeavesTimeOrdering(t *testing.T) {
	ctx := context.Background()

	// Queue two small batches of leaves at different timestamps. Do two separate dequeue
	// transactions and make sure the returned leaves are respecting the time ordering of the
	// queue.
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	batchSize := 2
	leaves := createTestLeaves(int64(batchSize), 0)
	leaves2 := createTestLeaves(int64(batchSize), int64(batchSize))

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()
		if _, err := tx.QueueLeaves(ctx, leaves, fakeQueueTime); err != nil {
			t.Fatalf("QueueLeaves(1st batch) = %v", err)
		}
		// These are one second earlier so should be dequeued first
		if _, err := tx.QueueLeaves(ctx, leaves2, fakeQueueTime.Add(-time.Second)); err != nil {
			t.Fatalf("QueueLeaves(2nd batch) = %v", err)
		}
		testonly.Commit(tx, t)
	}

	{
		// Now try to dequeue two leaves and we should get the second batch
		tx2 := testonly.BeginLogTx(s, logID, t)
		defer tx2.Close()
		dequeue1, err := tx2.DequeueLeaves(ctx, batchSize, fakeQueueTime)
		if err != nil {
			t.Fatalf("DequeueLeaves(1st) = %v", err)
		}
		if got, want := len(dequeue1), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (1st) got: %d, want: %d", got, want)
		}
		testonly.EnsureAllLeavesDistinct(dequeue1, t)

		// Ensure this is the second batch queued by comparing leaf hashes (must be distinct as
		// the leaf data was).
		if !testonly.LeafInBatch(dequeue1[0], leaves2) || !testonly.LeafInBatch(dequeue1[1], leaves2) {
			t.Fatalf("Got leaf from wrong batch (1st dequeue): %v", dequeue1)
		}
		testonly.Commit(tx2, t)

		// Try to dequeue again and we should get the batch that was queued first, though at a later time
		tx3 := testonly.BeginLogTx(s, logID, t)
		defer tx3.Close()
		dequeue2, err := tx3.DequeueLeaves(ctx, batchSize, fakeQueueTime)
		if err != nil {
			t.Fatalf("DequeueLeaves(2nd) = %v", err)
		}
		if got, want := len(dequeue2), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (2nd) got: %d, want: %d", got, want)
		}
		testonly.EnsureAllLeavesDistinct(dequeue2, t)

		// Ensure this is the first batch by comparing leaf hashes.
		if !testonly.LeafInBatch(dequeue2[0], leaves) || !testonly.LeafInBatch(dequeue2[1], leaves) {
			t.Fatalf("Got leaf from wrong batch (2nd dequeue): %v", dequeue2)
		}
		testonly.Commit(tx3, t)
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	hashes := [][]byte{[]byte("thisdoesn'texist")}
	leaves, err := tx.GetLeavesByHash(ctx, hashes, false)
	if err != nil {
		t.Fatalf("Error getting leaves by hash: %v", err)
	}
	if len(leaves) != 0 {
		t.Fatalf("Expected no leaves returned but got %d", len(leaves))
	}
	testonly.Commit(tx, t)
}

func TestGetLeavesByIndexNotPresent(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	if _, err := tx.GetLeavesByIndex(ctx, []int64{99999}); err == nil {
		t.Fatalf("Returned ok for leaf index when nothing inserted: %v", err)
	}
	testonly.Commit(tx, t)
}

func TestGetLeavesByHash(t *testing.T) {
	ctx := context.Background()

	// Create fake leaf as if it had been sequenced
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	data := []byte("some data")
	createFakeLeaf(ctx, dbWrapper.DB(), logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	hashes := [][]byte{dummyHash}
	leaves, err := tx.GetLeavesByHash(ctx, hashes, false)
	if err != nil {
		t.Fatalf("Unexpected error getting leaf by hash: %v", err)
	}
	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}
	testonly.CheckLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
	testonly.Commit(tx, t)
}

func TestGetLeafDataByIdentityHash(t *testing.T) {
	ctx := context.Background()

	// Create fake leaf as if it had been sequenced
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)
	data := []byte("some data")
	leaf := createFakeLeaf(ctx, dbWrapper.DB(), logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)
	leaf.LeafIndex = -1
	leaf.MerkleLeafHash = []byte(dummyMerkleLeafHash)
	leaf2 := createFakeLeaf(ctx, dbWrapper.DB(), logID, dummyHash2, dummyHash2, data, someExtraData, sequenceNumber+1, t)
	leaf2.LeafIndex = -1
	leaf2.MerkleLeafHash = []byte(dummyMerkleLeafHash)

	var tests = []struct {
		hashes [][]byte
		want   []*trillian.LogLeaf
	}{
		{
			hashes: [][]byte{dummyRawHash},
			want:   []*trillian.LogLeaf{leaf},
		},
		{
			hashes: [][]byte{{0x01, 0x02}},
		},
		{
			hashes: [][]byte{
				dummyRawHash,
				{0x01, 0x02},
				dummyHash2,
				{0x01, 0x02},
			},
			// Note: leaves not necessarily returned in order requested.
			want: []*trillian.LogLeaf{leaf2, leaf},
		},
	}
	for _, test := range tests {
		func() {
			tx := testonly.BeginLogTx(s, logID, t)
			defer tx.Close()

			leaves, err := tx.GetLeafDataByIdentityHash(ctx, test.hashes)
			if err != nil {
				t.Errorf("getLeavesByIdentityHash(_) = (_,%v); want (_,nil)", err)
				return
			}
			testonly.Commit(tx, t)
			if len(leaves) != len(test.want) {
				t.Errorf("getLeavesByIdentityHash(_) = (|%d|,nil); want (|%d|,nil)", len(leaves), len(test.want))
				return
			}
			for i, want := range test.want {
				if !proto.Equal(leaves[i], want) {
					diff := pretty.Compare(leaves[i], want)
					t.Errorf("getLeavesByIdentityHash(_)[%d] diff:\n%v", i, diff)
				}
			}
		}()
	}
}
func TestGetLeavesByIndex(t *testing.T) {
	ctx := context.Background()

	// Create fake leaf as if it had been sequenced, read it back and check contents
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	data := []byte("some data")
	createFakeLeaf(ctx, dbWrapper.DB(), logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)
	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	leaves, err := tx.GetLeavesByIndex(ctx, []int64{sequenceNumber})
	if err != nil {
		t.Fatalf("Unexpected error getting leaf by index: %v", err)
	}
	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}
	testonly.CheckLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
	testonly.Commit(tx, t)
}

func TestLatestSignedRootNoneWritten(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		t.Fatalf("Failed to read an empty log root: %v", err)
	}
	if root.LogId != 0 || len(root.RootHash) != 0 || root.Signature != nil {
		t.Fatalf("Read a root with contents when it should be empty: %v", root)
	}
	testonly.Commit(tx, t)
}

func TestLatestSignedLogRoot(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	testonly.Commit(tx, t)

	{
		tx2 := testonly.BeginLogTx(s, logID, t)
		defer tx2.Close()
		root2, err := tx2.LatestSignedLogRoot(ctx)
		if err != nil {
			t.Fatalf("Failed to read back new log root: %v", err)
		}
		if !proto.Equal(&root, &root2) {
			t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
		}
		testonly.Commit(tx2, t)
	}
}

func TestDuplicateSignedLogRoot(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()

	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	// Shouldn't be able to do it again
	if err := tx.StoreSignedLogRoot(ctx, root); err == nil {
		t.Fatal("Allowed duplicate signed root")
	}
	testonly.Commit(tx, t)
}

func TestLogRootUpdate(t *testing.T) {
	ctx := context.Background()

	// Write two roots for a log and make sure the one with the newest timestamp supersedes
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx := testonly.BeginLogTx(s, logID, t)
	defer tx.Close()
	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	root2 := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98766,
		TreeSize:       16,
		TreeRevision:   6,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(ctx, root2); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	testonly.Commit(tx, t)

	tx2 := testonly.BeginLogTx(s, logID, t)
	defer tx2.Close()
	root3, err := tx2.LatestSignedLogRoot(ctx)
	if err != nil {
		t.Fatalf("Failed to read back new log root: %v", err)
	}
	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
	}
	testonly.Commit(tx2, t)
}

// getActiveLogIDsFn creates a TX, calls the appropriate GetActiveLogIDs* function, commits the TX
// and returns the results.
type getActiveLogIDsFn func(context.Context, storage.LogStorage, int64) ([]int64, error)

type getActiveIDsTest struct {
	name string
	fn   getActiveLogIDsFn
}

func toIDsMap(ids []int64) map[int64]bool {
	idsMap := make(map[int64]bool)
	for _, logID := range ids {
		idsMap[logID] = true
	}
	return idsMap
}

// runTestGetActiveLogIDsInternal calls test.fn (which is either GetActiveLogIDs or
// GetActiveLogIDsWithPendingWork) and check that the result matches wantIds.
func runTestGetActiveLogIDsInternal(ctx context.Context, t *testing.T, test getActiveIDsTest, logID int64, wantIds []int64) {
	s := coresql.NewLogStorage(dbWrapper)
	logIDs, err := test.fn(s, context.Background(), logID)
	if err != nil {
		t.Errorf("%v = (_, %v), want = (_, nil)", test.name, err)
		return
	}

	if got, want := len(logIDs), len(wantIds); got != want {
		t.Errorf("%v: got %d IDs, want = %v", test.name, got, want)
		return
	}
	got, want := toIDsMap(logIDs), toIDsMap(wantIds)
	if diff := pretty.Compare(got, want); diff != "" {
		t.Errorf("%v: post-Get diff:\n%v", test.name, diff)
	}
}

func runTestGetActiveLogIDs(ctx context.Context, t *testing.T, test getActiveIDsTest) {
	cleanTestDB(dbWrapper)
	logID1 := testonly.CreateLogForTest(dbWrapper)
	logID2 := testonly.CreateLogForTest(dbWrapper)
	logID3 := testonly.CreateLogForTest(dbWrapper)
	wantIds := []int64{logID1, logID2, logID3}
	runTestGetActiveLogIDsInternal(ctx, t, test, logID1, wantIds)
}

func runTestGetActiveLogIDsWithPendingWork(ctx context.Context, t *testing.T, test getActiveIDsTest) {
	cleanTestDB(dbWrapper)
	logID1 := testonly.CreateLogForTest(dbWrapper)
	logID2 := testonly.CreateLogForTest(dbWrapper)
	logID3 := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	// Do a first run without any pending logs
	runTestGetActiveLogIDsInternal(ctx, t, test, logID1, nil)

	for _, logID := range []int64{logID1, logID2, logID3} {
		func() {
			tx := testonly.BeginLogTx(s, logID, t)
			defer tx.Close()
			leaves := createTestLeaves(leavesToInsert, 2)
			if _, err := tx.QueueLeaves(ctx, leaves, fakeQueueTime); err != nil {
				t.Fatalf("Failed to queue leaves for log %v: %v", logID, err)
			}
			testonly.Commit(tx, t)
		}()
	}

	wantIds := []int64{logID1, logID2, logID3}
	runTestGetActiveLogIDsInternal(ctx, t, test, logID1, wantIds)
}

func TestGetActiveLogIDs(t *testing.T) {
	getActiveIDsBegin := func(ctx context.Context, s storage.LogStorage, logID int64) ([]int64, error) {
		tx, err := s.BeginForTree(ctx, logID)
		if err != nil {
			return nil, err
		}
		defer tx.Close()
		ids, err := tx.GetActiveLogIDs(ctx)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	getActiveIDsSnapshot := func(ctx context.Context, s storage.LogStorage, logID int64) ([]int64, error) {
		tx, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Close()
		ids, err := tx.GetActiveLogIDs(ctx)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}

	tests := []getActiveIDsTest{
		{name: "getActiveIDsBegin", fn: getActiveIDsBegin},
		{name: "getActiveIDsSnapshot", fn: getActiveIDsSnapshot},
	}

	ctx := context.Background()
	for _, test := range tests {
		runTestGetActiveLogIDs(ctx, t, test)
	}
}

func TestGetActiveLogIDsEmpty(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	tx, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()

	activeIDs, err := tx.GetActiveLogIDs(ctx)
	if err != nil {
		t.Fatalf("GetActiveLogIDs() = (_, %v), want = (_, nil)", err)
	}
	if got, want := len(activeIDs), 0; got != want {
		t.Errorf("GetActiveLogIDs(): got %v IDs, want = %v", got, want)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() = %v, want = nil", err)
	}
}

func TestGetActiveLogIDsWithPendingWork(t *testing.T) {
	getActiveIDsBegin := func(ctx context.Context, s storage.LogStorage, logID int64) ([]int64, error) {
		tx, err := s.BeginForTree(ctx, logID)
		if err != nil {
			return nil, err
		}
		defer tx.Close()
		ids, err := tx.GetActiveLogIDsWithPendingWork(ctx)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	getActiveIDsSnapshot := func(ctx context.Context, s storage.LogStorage, logID int64) ([]int64, error) {
		tx, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Close()
		ids, err := tx.GetActiveLogIDsWithPendingWork(ctx)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}

	tests := []getActiveIDsTest{
		{name: "getActiveIDsBegin", fn: getActiveIDsBegin},
		{name: "getActiveIDsSnapshot", fn: getActiveIDsSnapshot},
	}
	ctx := context.Background()
	for _, test := range tests {
		runTestGetActiveLogIDsWithPendingWork(ctx, t, test)
	}
}

func TestReadOnlyLogTX_Rollback(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)
	tx, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()
	if _, err := tx.GetActiveLogIDs(ctx); err != nil {
		t.Fatalf("GetActiveLogIDs() = (_, %v), want = (_, nil)", err)
	}
	// It's a bit hard to have a more meaningful test. This should suffice.
	if err := tx.Rollback(); err != nil {
		t.Errorf("Rollback() = (_, %v), want = (_, nil)", err)
	}
}

func TestGetSequencedLeafCount(t *testing.T) {
	ctx := context.Background()

	// We'll create leaves for two different trees
	cleanTestDB(dbWrapper)
	logID1 := testonly.CreateLogForTest(dbWrapper)
	logID2 := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	{
		// Create fake leaf as if it had been sequenced
		data := []byte("some data")
		createFakeLeaf(ctx, dbWrapper.DB(), logID1, dummyHash, dummyRawHash, data, someExtraData, sequenceNumber, t)

		// Create fake leaves for second tree as if they had been sequenced
		data2 := []byte("some data 2")
		data3 := []byte("some data 3")
		createFakeLeaf(ctx, dbWrapper.DB(), logID2, dummyHash2, dummyRawHash, data2, someExtraData, sequenceNumber, t)
		createFakeLeaf(ctx, dbWrapper.DB(), logID2, dummyHash3, dummyRawHash, data3, someExtraData, sequenceNumber+1, t)
	}

	// Read back the leaf counts from both trees
	tx := testonly.BeginLogTx(s, logID1, t)
	defer tx.Close()
	count1, err := tx.GetSequencedLeafCount(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting leaf count: %v", err)
	}
	if want, got := int64(1), count1; want != got {
		t.Fatalf("expected %d sequenced for logId but got %d", want, got)
	}
	testonly.Commit(tx, t)

	tx = testonly.BeginLogTx(s, logID2, t)
	defer tx.Close()
	count2, err := tx.GetSequencedLeafCount(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting leaf count2: %v", err)
	}
	if want, got := int64(2), count2; want != got {
		t.Fatalf("expected %d sequenced for logId2 but got %d", want, got)
	}
	testonly.Commit(tx, t)
}

// Creates some test leaves with predictable data
func createTestLeaves(n, startSeq int64) []*trillian.LogLeaf {
	var leaves []*trillian.LogLeaf
	for l := int64(0); l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l+startSeq)
		h := sha256.New()
		h.Write([]byte(lv))
		leafHash := h.Sum(nil)
		leaf := &trillian.LogLeaf{
			LeafIdentityHash: leafHash,
			MerkleLeafHash:   leafHash,
			LeafValue:        []byte(lv),
			ExtraData:        []byte(fmt.Sprintf("Extra %d", l)),
			LeafIndex:        int64(startSeq + l),
		}
		leaves = append(leaves, leaf)
	}

	return leaves
}

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
	"runtime/debug"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"reflect"
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

func createFakeLeaf(db *sql.DB, logID int64, rawHash, hash, data, extraData []byte, seq int64, t *testing.T) {
	_, err := db.Exec("INSERT INTO LeafData(TreeId, LeafIdentityHash, LeafValue, ExtraData) VALUES(?,?,?,?)", logID, rawHash, data, extraData)
	_, err2 := db.Exec("INSERT INTO SequencedLeafData(TreeId, SequenceNumber, LeafIdentityHash, MerkleLeafHash) VALUES(?,?,?,?)", logID, seq, rawHash, hash)

	if err != nil || err2 != nil {
		t.Fatalf("Failed to create test leaves: %v %v", err, err2)
	}
}

func checkLeafContents(leaf trillian.LogLeaf, seq int64, rawHash, hash, data, extraData []byte, t *testing.T) {
	if got, want := leaf.MerkleLeafHash, hash; !bytes.Equal(got, want) {
		t.Fatalf("Wrong leaf hash in returned leaf got\n%v\nwant:\n%v", got, want)
	}

	if got, want := leaf.LeafIdentityHash, rawHash; !bytes.Equal(got, want) {
		t.Fatalf("Wrong raw leaf hash in returned leaf got\n%v\nwant:\n%v", got, want)
	}

	if got, want := seq, leaf.LeafIndex; got != want {
		t.Fatalf("Bad sequence number in returned leaf got: %d, want:%d", got, want)
	}

	if got, want := leaf.LeafValue, data; !bytes.Equal(got, want) {
		t.Fatalf("Unxpected data in returned leaf. got:\n%v\nwant:\n%v", got, want)
	}

	if got, want := leaf.ExtraData, extraData; !bytes.Equal(got, want) {
		t.Fatalf("Unxpected data in returned leaf. got:\n%v\nwant:\n%v", got, want)
	}
}

func setAllowsDuplicates(db *sql.DB, treeID int64, allowDuplicates bool) error {
	stmt, err := db.Prepare("UPDATE Trees SET AllowsDuplicateLeaves = ? WHERE TreeId = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(allowDuplicates, treeID)
	return err
}

func TestBegin(t *testing.T) {
	logID1 := createLogID("TestBegin1")
	logID2 := createLogID("TestBegin2")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID1, t)
	prepareTestLogDB(DB, logID2, t)

	storage, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v)", err)
	}

	tests := []struct {
		logID           int64
		err             string
		allowDuplicates bool
		writeRevision   int
	}{
		{logID: -1, err: "failed to get tree row"},
		{logID: logID1.logID, allowDuplicates: true},
		{logID: logID2.logID, allowDuplicates: false},
	}

	ctx := context.TODO()
	for _, test := range tests {
		if test.allowDuplicates {
			if err := setAllowsDuplicates(DB, test.logID, test.allowDuplicates); err != nil {
				t.Fatalf("setup error: cannot set allowDuplicates on DB: %v", err)
			}
		}

		tx, err := storage.BeginForTree(ctx, test.logID)
		if hasError, wantError := err != nil, test.err != ""; hasError || wantError {
			if hasError != wantError || (wantError && !strings.Contains(err.Error(), test.err)) {
				t.Errorf("Begin() = (_, '%v'), want = (_, '...%v...')", err, test.err)
			}
			continue
		}

		// TODO(codingllama): It would be better to test this via side effects of other public methods
		if tx.(*logTreeTX).allowDuplicates != test.allowDuplicates {
			t.Errorf("tx.allowDuplicates = %v, want = %v", tx.(*logTreeTX).allowDuplicates, test.allowDuplicates)
		}

		root, err := tx.LatestSignedLogRoot()
		if err != nil {
			t.Errorf("LatestSignedLogRoot() = (_, %v), want = (_, nil)", err)
		}

		if got, want := tx.WriteRevision(), root.TreeRevision+1; got != want {
			t.Errorf("WriteRevision() = %v, want = %v", got, want)
		}

		if err := tx.Commit(); err != nil {
			t.Errorf("Commit() = %v, want = nil", err)
		}

	}

}

func TestSnapshot(t *testing.T) {
	logID := createLogID("TestSnapshot")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)

	storage, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v)", err)
	}

	tests := []struct {
		logID int64
		err   string
	}{
		{logID: -1, err: "failed to get tree row"},
		{logID: logID.logID},
	}

	ctx := context.TODO()
	for _, test := range tests {
		tx, err := storage.SnapshotForTree(ctx, test.logID)
		if hasError, wantError := err != nil, test.err != ""; hasError || wantError {
			if hasError != wantError || (wantError && !strings.Contains(err.Error(), test.err)) {
				t.Errorf("Begin() = (_, '%v'), want = (_, '...%v...')", err, test.err)
			}
			continue
		}

		// Do a read so we have something to commit on the snapshot
		_, err = tx.LatestSignedLogRoot()
		if err != nil {
			t.Errorf("LatestSignedLogRoot() = (_, %v), want = (_, nil)", err)
		}

		if err := tx.Commit(); err != nil {
			t.Errorf("Commit() = %v, want = nil", err)
		}

	}

}

func TestOpenStateCommit(t *testing.T) {
	logID := createLogID("TestOpenStateCommit")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	ctx := context.Background()

	tx, err := s.BeginForTree(ctx, logID.logID)
	if err != nil {
		t.Fatalf("Failed to set up db transaction: %v", err)
	}

	if !tx.IsOpen() {
		t.Fatal("Transaction should be open on creation")
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if tx.IsOpen() {
		t.Fatal("Transaction should be closed after commit")
	}
}

func TestOpenStateRollback(t *testing.T) {
	logID := createLogID("TestOpenStateRollback")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	ctx := context.Background()

	tx, err := s.BeginForTree(ctx, logID.logID)
	if err != nil {
		t.Fatalf("Failed to set up db transaction: %v", err)
	}

	if !tx.IsOpen() {
		t.Fatal("Transaction should be open on creation")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if tx.IsOpen() {
		t.Fatal("Transaction should be closed after rollback")
	}
}

func TestQueueDuplicateLeafFails(t *testing.T) {
	logID := createLogID("TestQueueDuplicateLeafFails")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	leaves := createTestLeaves(5, 10)

	if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	leaves2 := createTestLeaves(5, 12)

	if err := tx.QueueLeaves(leaves2, fakeQueueTime); err == nil {
		t.Fatal("Allowed duplicate leaves to be inserted")

		if !strings.Contains(err.Error(), "Duplicate") {
			t.Fatalf("Got the wrong type of error: %v", err)
		}
	}
}

func TestQueueLeaves(t *testing.T) {
	logID := createLogID("TestQueueLeaves")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer failIfTXStillOpen(t, "TestQueueLeaves", tx)

	leaves := createTestLeaves(leavesToInsert, 20)

	if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the
	// unsequenced data.
	var count int

	if err := DB.QueryRow("SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID.logID).Scan(&count); err != nil {
		t.Fatalf("Could not query row count: %v", err)
	}

	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}

	// Additional check on timestamp being set correctly in the database
	var queueTimestamp int64
	if err := DB.QueryRow("SELECT DISTINCT QueueTimestampNanos FROM Unsequenced WHERE TreeID=?", logID.logID).Scan(&queueTimestamp); err != nil {
		t.Fatalf("Could not query timestamp: %v", err)
	}

	if got, want := queueTimestamp, fakeQueueTime.UnixNano(); got != want {
		t.Fatalf("Incorrect queue timestamp got: %d want: %d", got, want)
	}
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	logID := createLogID("TestDequeueLeavesNoneQueued")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	leaves, err := tx.DequeueLeaves(999, fakeDequeueCutoffTime)

	if err != nil {
		t.Fatalf("Didn't expect an error on dequeue with no work to be done: %v", err)
	}

	if len(leaves) > 0 {
		t.Fatalf("Expected nothing to be dequeued but we got %d leaves", len(leaves))
	}
}

func TestDequeueLeaves(t *testing.T) {
	logID := createLogID("TestDequeueLeaves")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	{
		tx := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeLeaves", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue them
		tx2 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeLeaves", tx2)
		leaves2, err := tx2.DequeueLeaves(99, fakeDequeueCutoffTime)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}

		if len(leaves2) != leavesToInsert {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}

		ensureAllLeavesDistinct(leaves2, t)
		commit(tx2, t)
	}

	{
		// If we dequeue again then we should now get nothing
		tx3 := beginLogTx(s, logID, t)
		defer tx3.Rollback()

		leaves3, err := tx3.DequeueLeaves(99, fakeDequeueCutoffTime)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}

		if len(leaves3) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves3))
		}
	}
}

func TestDequeueLeavesTwoBatches(t *testing.T) {
	logID := createLogID("TestDequeueLeavesTwoBatches")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	leavesToDequeue1 := 3
	leavesToDequeue2 := 2

	{
		tx := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue some of them
		tx2 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches-tx2", tx2)
		leaves2, err := tx2.DequeueLeaves(leavesToDequeue1, fakeDequeueCutoffTime)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}

		if len(leaves2) != leavesToDequeue1 {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}

		ensureAllLeavesDistinct(leaves2, t)
		commit(tx2, t)

		// Now try to dequeue the rest of them
		tx3 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches-tx3", tx3)
		leaves3, err := tx3.DequeueLeaves(leavesToDequeue2, fakeDequeueCutoffTime)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}

		if len(leaves3) != leavesToDequeue2 {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves3), leavesToDequeue2)
		}

		ensureAllLeavesDistinct(leaves3, t)

		// Plus the union of the leaf batches should all have distinct hashes
		leaves4 := append(leaves2, leaves3...)
		ensureAllLeavesDistinct(leaves4, t)

		commit(tx3, t)
	}

	{
		// If we dequeue again then we should now get nothing
		tx4 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches-tx4", tx4)

		leaves5, err := tx4.DequeueLeaves(99, fakeDequeueCutoffTime)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}

		if len(leaves5) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves5))
		}

		commit(tx4, t)
	}
}

// Queues leaves and attempts to dequeue before the guard cutoff allows it. This should
// return nothing. Then retry with an inclusive guard cutoff and ensure the leaves
// are returned.
func TestDequeueLeavesGuardInterval(t *testing.T) {
	logID := createLogID("TestDequeueLeavesGuardInterval")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	{
		tx := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesGuardInterval", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue them using a cutoff that means we should get none
		tx2 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesGuardInterval", tx2)
		leaves2, err := tx2.DequeueLeaves(99, fakeQueueTime.Add(-time.Second))

		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}

		if len(leaves2) != 0 {
			t.Fatalf("Dequeued %d leaves when they all should be in guard interval", len(leaves2))
		}

		// Try to dequeue again using a cutoff that should include them
		leaves2, err = tx2.DequeueLeaves(99, fakeQueueTime.Add(time.Second))

		if err != nil {
			t.Fatalf("Failed to dequeue leaves: %v", err)
		}

		if len(leaves2) != leavesToInsert {
			t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToInsert)
		}

		ensureAllLeavesDistinct(leaves2, t)
		commit(tx2, t)
	}
}

func TestDequeueLeavesTimeOrdering(t *testing.T) {
	// Queue two small batches of leaves at different timestamps. Do two separate dequeue
	// transactions and make sure the returned leaves are respecting the time ordering of the
	// queue.
	logID := createLogID("TestDequeueLeavesTimeOrdering")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	batchSize := 2

	{
		tx := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTimeOrdering", tx)

		leaves := createTestLeaves(int64(batchSize), 0)
		leaves2 := createTestLeaves(int64(batchSize), int64(batchSize))

		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("QueueLeaves(1st batch) = %v", err)
		}

		// These are one second earlier so should be dequeued first
		if err := tx.QueueLeaves(leaves2, fakeQueueTime.Add(-time.Second)); err != nil {
			t.Fatalf("QueueLeaves(2nd batch) = %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue two leaves and we should get the second batch
		tx2 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTimeOrdering", tx2)
		dequeue1, err := tx2.DequeueLeaves(batchSize, fakeQueueTime)

		if err != nil {
			t.Fatalf("DequeueLeaves(1st) = %v", err)
		}

		if got, want := len(dequeue1), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (1st) got: %d, want: %d", got, want)
		}

		ensureAllLeavesDistinct(dequeue1, t)

		// Ensure this is the second batch queued by comparing leaf data.
		if !leafInRange(dequeue1[0], batchSize, batchSize+batchSize-1) || !leafInRange(dequeue1[1], batchSize, batchSize+batchSize-1) {
			t.Fatalf("Got leaf from wrong batch (1st dequeue): (%s %s)", string(dequeue1[0].LeafValue), string(dequeue1[1].LeafValue))
		}

		commit(tx2, t)

		// Try to dequeue again and we should get the batch that was queued first, though at a later time
		tx3 := beginLogTx(s, logID, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTimeOrdering", tx3)
		dequeue2, err := tx3.DequeueLeaves(batchSize, fakeQueueTime)

		if err != nil {
			t.Fatalf("DequeueLeaves(2nd) = %v", err)
		}

		if got, want := len(dequeue2), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (2nd) got: %d, want: %d", got, want)
		}

		ensureAllLeavesDistinct(dequeue2, t)

		// Ensure this is the first batch by comparing leaf data.
		if !leafInRange(dequeue2[0], 0, batchSize-1) || !leafInRange(dequeue2[1], 0, batchSize-1) {
			t.Fatalf("Got leaf from wrong batch (2nd dequeue): (%s %s)", string(dequeue2[0].LeafValue), string(dequeue2[1].LeafValue))
		}

		commit(tx3, t)
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByHashNotPresent")
	cleanTestDB(DB)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	hashes := [][]byte{[]byte("thisdoesn'texist")}
	leaves, err := tx.GetLeavesByHash(hashes, false)

	if err != nil {
		t.Fatalf("Error getting leaves by hash: %v", err)
	}

	if len(leaves) != 0 {
		t.Fatalf("Expected no leaves returned but got %d", len(leaves))
	}
}

func TestGetLeavesByIndexNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByIndexNotPresent")
	cleanTestDB(DB)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	_, err := tx.GetLeavesByIndex([]int64{99999})

	if err == nil {
		t.Fatalf("Returned ok for leaf index when nothing inserted: %v", err)
	}
}

func TestGetLeavesByHash(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByHash")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	data := []byte("some data")

	createFakeLeaf(DB, logID.logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	hashes := [][]byte{dummyHash}
	leaves, err := tx.GetLeavesByHash(hashes, false)

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by hash: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced, read it back and check contents
	logID := createLogID("TestGetLeavesByIndex")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	data := []byte("some data")
	createFakeLeaf(DB, logID.logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	leaves, err := tx.GetLeavesByIndex([]int64{sequenceNumber})

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by index: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
}

func TestLatestSignedRootNoneWritten(t *testing.T) {
	logID := createLogID("TestLatestSignedLogRootNoneWritten")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer tx.Rollback()

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		t.Fatalf("Failed to read an empty log root: %v", err)
	}

	if root.LogId != 0 || len(root.RootHash) != 0 || root.Signature != nil {
		t.Fatalf("Read a root with contents when it should be empty: %v", root)
	}
}

func TestLatestSignedLogRoot(t *testing.T) {
	logID := createLogID("TestLatestSignedLogRoot")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer tx.Rollback()

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	commit(tx, t)

	{
		tx2 := beginLogTx(s, logID, t)
		defer tx2.Rollback()
		root2, err := tx2.LatestSignedLogRoot()

		if err != nil {
			t.Fatalf("Failed to read back new log root: %v", err)
		}

		if !proto.Equal(&root, &root2) {
			t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
		}
	}
}

func TestDuplicateSignedLogRoot(t *testing.T) {
	logID := createLogID("TestDuplicateSignedLogRoot")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)
	defer commit(tx, t)

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	// Shouldn't be able to do it again
	if err := tx.StoreSignedLogRoot(root); err == nil {
		t.Fatal("Allowed duplicate signed root")
	}
}

func TestLogRootUpdate(t *testing.T) {
	// Write two roots for a log and make sure the one with the newest timestamp supersedes
	logID := createLogID("TestLatestSignedLogRoot")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)
	tx := beginLogTx(s, logID, t)

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root2 := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98766, TreeSize: 16, TreeRevision: 6, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root2); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	commit(tx, t)

	tx2 := beginLogTx(s, logID, t)
	defer commit(tx2, t)
	root3, err := tx2.LatestSignedLogRoot()

	if err != nil {
		t.Fatalf("Failed to read back new log root: %v", err)
	}

	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
	}
}

// getActiveLogIDsFn creates a TX, calls the appropriate GetActiveLogIDs* function, commits the TX
// and returns the results.
type getActiveLogIDsFn func(storage.LogStorage, context.Context, int64) ([]int64, error)

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
func runTestGetActiveLogIDsInternal(t *testing.T, test getActiveIDsTest, logID int64, wantIds []int64) {
	s, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v), want = (_, nil)", err)
	}

	logIDs, err := test.fn(s, context.TODO(), logID)
	if err != nil {
		t.Errorf("%v = (_, %v), want = (_, nil)", test.name, err)
		return
	}

	if got, want := len(logIDs), len(wantIds); got != want {
		t.Errorf("%v: got %d IDs, want = %v", test.name, got, want)
		return
	}
	if got, want := toIDsMap(logIDs), toIDsMap(wantIds); !reflect.DeepEqual(got, want) {
		t.Errorf("%v = (%v, _), want = (%v, _)", test.name, got, want)
	}
}

func runTestGetActiveLogIDs(t *testing.T, test getActiveIDsTest) {
	logID1 := createLogID("TestGetActiveLogIDs1")
	logID2 := createLogID("TestGetActiveLogIDs2")
	logID3 := createLogID("TestGetActiveLogIDs3")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID1, t)
	prepareTestLogDB(DB, logID2, t)
	prepareTestLogDB(DB, logID3, t)

	wantIds := []int64{logID1.logID, logID2.logID, logID3.logID}
	runTestGetActiveLogIDsInternal(t, test, logID1.logID, wantIds)
}

func runTestGetActiveLogIDsWithPendingWork(t *testing.T, test getActiveIDsTest) {
	logID1 := createLogID("TestGetActiveLogIDsWithPendingWork1")
	logID2 := createLogID("TestGetActiveLogIDsWithPendingWork2")
	logID3 := createLogID("TestGetActiveLogIDsWithPendingWork3")
	cleanTestDB(DB)
	prepareTestLogDB(DB, logID1, t)
	prepareTestLogDB(DB, logID2, t)
	prepareTestLogDB(DB, logID3, t)

	s, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v), want = (_, nil)", err)
	}

	// Do a first run without any pending logs
	runTestGetActiveLogIDsInternal(t, test, logID1.logID, nil)

	for _, logID := range []logIDAndTest{logID1, logID2, logID3} {
		tx := beginLogTx(s, logID, t)
		leaves := createTestLeaves(leavesToInsert, 2)
		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("failed to queue leaves for log %v: %v", logID.logID, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("failed to Commit leaves for log %v: %v", logID.logID, err)
		}
	}

	wantIds := []int64{logID1.logID, logID2.logID, logID3.logID}
	runTestGetActiveLogIDsInternal(t, test, logID1.logID, wantIds)
}

func TestGetActiveLogIDs(t *testing.T) {
	getActiveIDsBegin := func(s storage.LogStorage, ctx context.Context, logID int64) ([]int64, error) {
		tx, err := s.BeginForTree(ctx, logID)
		if err != nil {
			return nil, err
		}
		ids, err := tx.GetActiveLogIDs()
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	getActiveIDsSnapshot := func(s storage.LogStorage, ctx context.Context, logID int64) ([]int64, error) {
		tx, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		ids, err := tx.GetActiveLogIDs()
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
	for _, test := range tests {
		runTestGetActiveLogIDs(t, test)
	}
}

func TestGetActiveLogIDsEmpty(t *testing.T) {
	cleanTestDB(DB)

	s, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v), want = (_, nil)", err)
	}

	tx, err := s.Snapshot(context.TODO())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}

	activeIDs, err := tx.GetActiveLogIDs()
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
	getActiveIDsBegin := func(s storage.LogStorage, ctx context.Context, logID int64) ([]int64, error) {
		tx, err := s.BeginForTree(ctx, logID)
		if err != nil {
			return nil, err
		}
		ids, err := tx.GetActiveLogIDsWithPendingWork()
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	getActiveIDsSnapshot := func(s storage.LogStorage, ctx context.Context, logID int64) ([]int64, error) {
		tx, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		ids, err := tx.GetActiveLogIDsWithPendingWork()
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
	for _, test := range tests {
		runTestGetActiveLogIDsWithPendingWork(t, test)
	}
}

func TestReadOnlyLogTX_Rollback(t *testing.T) {
	cleanTestDB(DB)

	s, err := NewLogStorage(DB)
	if err != nil {
		t.Fatalf("NewLogStorage() = (_, %v), want = (_, nil)", err)
	}

	tx, err := s.Snapshot(context.TODO())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}

	if _, err := tx.GetActiveLogIDs(); err != nil {
		t.Fatalf("GetActiveLogIDs() = (_, %v), want = (_, nil)", err)
	}

	// It's a bit hard to have a more meaningful test. This should suffice.
	if err := tx.Rollback(); err != nil {
		t.Errorf("Rollback() = (_, %v), want = (_, nil)", err)
	}
}

func TestGetSequencedLeafCount(t *testing.T) {
	// We'll create leaves for two different trees
	logID := createLogID("TestGetSequencedLeafCount")
	logID2 := createLogID("TestGetSequencedLeafCount2")
	cleanTestDB(DB)
	s1 := prepareTestLogStorage(DB, logID, t)
	s2 := prepareTestLogStorage(DB, logID2, t)
	{
		// Create fake leaf as if it had been sequenced
		prepareTestLogDB(DB, logID, t)
		data := []byte("some data")
		createFakeLeaf(DB, logID.logID, dummyHash, dummyRawHash, data, someExtraData, sequenceNumber, t)

		// Create fake leaves for second tree as if they had been sequenced
		prepareTestLogDB(DB, logID2, t)
		data2 := []byte("some data 2")
		data3 := []byte("some data 3")
		createFakeLeaf(DB, logID2.logID, dummyHash2, dummyRawHash, data2, someExtraData, sequenceNumber, t)
		createFakeLeaf(DB, logID2.logID, dummyHash3, dummyRawHash, data3, someExtraData, sequenceNumber+1, t)
	}

	// Read back the leaf counts from both trees
	tx := beginLogTx(s1, logID, t)
	count1, err := tx.GetSequencedLeafCount()
	commit(tx, t)

	if err != nil {
		t.Fatalf("unexpected error getting leaf count: %v", err)
	}

	if want, got := int64(1), count1; want != got {
		t.Fatalf("expected %d sequenced for logId but got %d", want, got)
	}

	tx = beginLogTx(s2, logID2, t)
	count2, err := tx.GetSequencedLeafCount()
	commit(tx, t)

	if err != nil {
		t.Fatalf("unexpected error getting leaf count2: %v", err)
	}

	if want, got := int64(2), count2; want != got {
		t.Fatalf("expected %d sequenced for logId2 but got %d", want, got)
	}
}

func ensureAllLeavesDistinct(leaves []trillian.LogLeaf, t *testing.T) {
	// All the leaf value hashes should be distinct because the leaves were created with distinct
	// leaf data. If only we had maps with slices as keys or sets or pretty much any kind of usable
	// data structures we could do this properly.
	for i := range leaves {
		for j := range leaves {
			if i != j && bytes.Equal(leaves[i].LeafIdentityHash, leaves[j].LeafIdentityHash) {
				t.Fatalf("Unexpectedly got a duplicate leaf hash: %v %v",
					leaves[i].LeafIdentityHash, leaves[j].LeafIdentityHash)
			}
		}
	}
}

// Creates some test leaves with predictable data
func createTestLeaves(n, startSeq int64) []trillian.LogLeaf {
	var leaves []trillian.LogLeaf
	for l := int64(0); l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l+startSeq)
		h := sha256.New()
		h.Write([]byte(lv))
		leafHash := h.Sum(nil)
		leaf := trillian.LogLeaf{
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

// Convenience methods to avoid copying out "if err != nil { blah }" all over the place
func commit(tx storage.LogTreeTX, t *testing.T) {
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit tx: %v", err)
	}
}

func beginLogTx(s storage.LogStorage, logID logIDAndTest, t *testing.T) storage.LogTreeTX {
	tx, err := s.BeginForTree(context.Background(), logID.logID)

	if err != nil {
		t.Fatalf("Failed to begin log tx: %v", err)
	}

	return tx
}

func failIfTXStillOpen(t *testing.T, op string, tx storage.LogTreeTX) {
	if r := recover(); r != nil {
		// Check for the test bailing with panic before testing for unclosed tx.
		// debug.Stack() does the right thing and includes the original failure point
		t.Fatalf("Panic in %s: %v %v", op, r, string(debug.Stack()))
	}

	if tx != nil && tx.IsOpen() {
		t.Fatalf("Unclosed transaction in : %s", op)
	}
}

func leafInRange(leaf trillian.LogLeaf, min, max int) bool {
	for l := min; l <= max; l++ {
		if string(leaf.LeafValue) == fmt.Sprintf("Leaf %d", l) {
			return true
		}
	}

	return false
}

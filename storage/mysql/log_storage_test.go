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
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
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

func checkLeafContents(leaf *trillian.LogLeaf, seq int64, rawHash, hash, data, extraData []byte, t *testing.T) {
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

// TODO(codingllama): Replace with a GetTree/UpdateTree sequence, when the latter is available.
func updateDuplicatePolicy(db *sql.DB, treeID int64, duplicatePolicy trillian.DuplicatePolicy) error {
	dbPolicy := ""
	for k, v := range duplicatePolicyMap {
		if v == duplicatePolicy {
			dbPolicy = k
			break
		}
	}
	if dbPolicy == "" {
		return fmt.Errorf("unknown DuplicatePolicy: %s", duplicatePolicy)
	}
	stmt, err := db.Prepare("UPDATE Trees SET DuplicatePolicy = ? WHERE TreeId = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(dbPolicy, treeID)
	return err
}

func TestMySQLLogStorage_CheckDatabaseAccessible(t *testing.T) {
	cleanTestDB(DB)
	s := NewLogStorage(DB)
	if err := s.CheckDatabaseAccessible(context.Background()); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func TestBegin(t *testing.T) {
	cleanTestDB(DB)
	logID1 := createLogForTests(DB)
	logID2 := createLogForTests(DB)
	storage := NewLogStorage(DB)

	tests := []struct {
		logID           int64
		err             string
		duplicatePolicy trillian.DuplicatePolicy
		writeRevision   int
	}{
		{logID: -1, err: "failed to get tree row"},
		{logID: logID1, duplicatePolicy: trillian.DuplicatePolicy_DUPLICATES_ALLOWED},
		{logID: logID2, duplicatePolicy: trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED},
	}

	ctx := context.Background()
	for _, test := range tests {
		if test.duplicatePolicy != trillian.DuplicatePolicy_UNKNOWN_DUPLICATE_POLICY {
			if err := updateDuplicatePolicy(DB, test.logID, test.duplicatePolicy); err != nil {
				t.Fatalf("cannot update DuplicatePolicy: %v", err)
			}
		}

		tx, err := storage.BeginForTree(ctx, test.logID)
		if hasError, wantError := err != nil, test.err != ""; hasError || wantError {
			if hasError != wantError || (wantError && !strings.Contains(err.Error(), test.err)) {
				t.Errorf("Begin() = (_, '%v'), want = (_, '...%v...')", err, test.err)
			}
			continue
		}
		defer tx.Close()

		// TODO(codingllama): It would be better to test this via side effects of other public methods
		if tx.(*logTreeTX).duplicatePolicy != test.duplicatePolicy {
			t.Errorf("tx.allowDuplicates = %s, want = %s", tx.(*logTreeTX).duplicatePolicy, test.duplicatePolicy)
		}
		root, err := tx.LatestSignedLogRoot()
		if err != nil {
			t.Errorf("LatestSignedLogRoot() = (_, %v), want = (_, nil)", err)
		}
		if got, want := tx.WriteRevision(), root.TreeRevision+1; got != want {
			t.Errorf("WriteRevision() = %v, want = %v", got, want)
		}
		commit(tx, t)
	}
}

func TestSnapshot(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tests := []struct {
		logID int64
		err   string
	}{
		{logID: -1, err: "failed to get tree row"},
		{logID: logID},
	}

	ctx := context.Background()
	for _, test := range tests {
		tx, err := s.SnapshotForTree(ctx, test.logID)
		if hasError, wantError := err != nil, test.err != ""; hasError || wantError {
			if hasError != wantError || (wantError && !strings.Contains(err.Error(), test.err)) {
				t.Errorf("Begin() = (_, '%v'), want = (_, '...%v...')", err, test.err)
			}
			continue
		}
		defer tx.Close()

		// Do a read so we have something to commit on the snapshot
		if _, err = tx.LatestSignedLogRoot(); err != nil {
			t.Errorf("LatestSignedLogRoot() = (_, %v), want = (_, nil)", err)
		}
		commit(tx, t)
	}
}

func TestIsOpenCommitRollbackClosed(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tests := []struct {
		commit, rollback, close bool
	}{
		{commit: true},
		{rollback: true},
		{close: true},
	}
	for _, test := range tests {
		tx := beginLogTx(s, logID, t)
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
			continue
		}
		if tx.IsOpen() {
			t.Errorf("Transaction should be closed after commit/rollback/close, test: %v", test)
		}
	}
}

func TestQueueDuplicateLeafFails(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	leaves := createTestLeaves(5, 10)
	if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}
	leaves2 := createTestLeaves(5, 12)
	if err := tx.QueueLeaves(leaves2, fakeQueueTime); err == nil {
		t.Fatal("Allowed duplicate leaves to be inserted")
	}
	commit(tx, t)
}

func TestQueueLeaves(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	leaves := createTestLeaves(leavesToInsert, 20)
	if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}
	commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the unsequenced data.
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID).Scan(&count); err != nil {
		t.Fatalf("Could not query row count: %v", err)
	}
	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}

	// Additional check on timestamp being set correctly in the database
	var queueTimestamp int64
	if err := DB.QueryRow("SELECT DISTINCT QueueTimestampNanos FROM Unsequenced WHERE TreeID=?", logID).Scan(&queueTimestamp); err != nil {
		t.Fatalf("Could not query timestamp: %v", err)
	}
	if got, want := queueTimestamp, fakeQueueTime.UnixNano(); got != want {
		t.Fatalf("Incorrect queue timestamp got: %d want: %d", got, want)
	}
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	leaves, err := tx.DequeueLeaves(999, fakeDequeueCutoffTime)
	if err != nil {
		t.Fatalf("Didn't expect an error on dequeue with no work to be done: %v", err)
	}
	if len(leaves) > 0 {
		t.Fatalf("Expected nothing to be dequeued but we got %d leaves", len(leaves))
	}
	commit(tx, t)
}

func TestDequeueLeaves(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	{
		tx := beginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		commit(tx, t)
	}

	{
		// Now try to dequeue them
		tx2 := beginLogTx(s, logID, t)
		defer tx2.Close()
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
		defer tx3.Close()
		leaves3, err := tx3.DequeueLeaves(99, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}
		if len(leaves3) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves3))
		}
		commit(tx3, t)
	}
}

func TestDequeueLeavesTwoBatches(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	leavesToDequeue1 := 3
	leavesToDequeue2 := 2

	{
		tx := beginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		commit(tx, t)
	}

	{
		// Now try to dequeue some of them
		tx2 := beginLogTx(s, logID, t)
		defer tx2.Close()
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
		defer tx3.Close()
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
		defer tx4.Close()
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
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	{
		tx := beginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 20)
		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}
		commit(tx, t)
	}

	{
		// Now try to dequeue them using a cutoff that means we should get none
		tx2 := beginLogTx(s, logID, t)
		defer tx2.Close()
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
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	batchSize := 2
	leaves := createTestLeaves(int64(batchSize), 0)
	leaves2 := createTestLeaves(int64(batchSize), int64(batchSize))

	{
		tx := beginLogTx(s, logID, t)
		defer tx.Close()
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
		defer tx2.Close()
		dequeue1, err := tx2.DequeueLeaves(batchSize, fakeQueueTime)
		if err != nil {
			t.Fatalf("DequeueLeaves(1st) = %v", err)
		}
		if got, want := len(dequeue1), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (1st) got: %d, want: %d", got, want)
		}
		ensureAllLeavesDistinct(dequeue1, t)

		// Ensure this is the second batch queued by comparing leaf hashes (must be distinct as
		// the leaf data was).
		if !leafInBatch(dequeue1[0], leaves2) || !leafInBatch(dequeue1[1], leaves2) {
			t.Fatalf("Got leaf from wrong batch (1st dequeue): %v", dequeue1)
		}
		commit(tx2, t)

		// Try to dequeue again and we should get the batch that was queued first, though at a later time
		tx3 := beginLogTx(s, logID, t)
		defer tx3.Close()
		dequeue2, err := tx3.DequeueLeaves(batchSize, fakeQueueTime)
		if err != nil {
			t.Fatalf("DequeueLeaves(2nd) = %v", err)
		}
		if got, want := len(dequeue2), batchSize; got != want {
			t.Fatalf("Dequeue count mismatch (2nd) got: %d, want: %d", got, want)
		}
		ensureAllLeavesDistinct(dequeue2, t)

		// Ensure this is the first batch by comparing leaf hashes.
		if !leafInBatch(dequeue2[0], leaves) || !leafInBatch(dequeue2[1], leaves) {
			t.Fatalf("Got leaf from wrong batch (2nd dequeue): %v", dequeue2)
		}
		commit(tx3, t)
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	hashes := [][]byte{[]byte("thisdoesn'texist")}
	leaves, err := tx.GetLeavesByHash(hashes, false)
	if err != nil {
		t.Fatalf("Error getting leaves by hash: %v", err)
	}
	if len(leaves) != 0 {
		t.Fatalf("Expected no leaves returned but got %d", len(leaves))
	}
	commit(tx, t)
}

func TestGetLeavesByIndexNotPresent(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	if _, err := tx.GetLeavesByIndex([]int64{99999}); err == nil {
		t.Fatalf("Returned ok for leaf index when nothing inserted: %v", err)
	}
	commit(tx, t)
}

func TestGetLeavesByHash(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	data := []byte("some data")
	createFakeLeaf(DB, logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	hashes := [][]byte{dummyHash}
	leaves, err := tx.GetLeavesByHash(hashes, false)
	if err != nil {
		t.Fatalf("Unexpected error getting leaf by hash: %v", err)
	}
	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}
	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
	commit(tx, t)
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced, read it back and check contents
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	data := []byte("some data")
	createFakeLeaf(DB, logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	leaves, err := tx.GetLeavesByIndex([]int64{sequenceNumber})
	if err != nil {
		t.Fatalf("Unexpected error getting leaf by index: %v", err)
	}
	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}
	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
	commit(tx, t)
}

func TestLatestSignedRootNoneWritten(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		t.Fatalf("Failed to read an empty log root: %v", err)
	}
	if root.LogId != 0 || len(root.RootHash) != 0 || root.Signature != nil {
		t.Fatalf("Read a root with contents when it should be empty: %v", root)
	}
	commit(tx, t)
}

func TestLatestSignedLogRoot(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	commit(tx, t)

	{
		tx2 := beginLogTx(s, logID, t)
		defer tx2.Close()
		root2, err := tx2.LatestSignedLogRoot()
		if err != nil {
			t.Fatalf("Failed to read back new log root: %v", err)
		}
		if !proto.Equal(&root, &root2) {
			t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
		}
		commit(tx2, t)
	}
}

func TestDuplicateSignedLogRoot(t *testing.T) {
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()

	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	// Shouldn't be able to do it again
	if err := tx.StoreSignedLogRoot(root); err == nil {
		t.Fatal("Allowed duplicate signed root")
	}
	commit(tx, t)
}

func TestLogRootUpdate(t *testing.T) {
	// Write two roots for a log and make sure the one with the newest timestamp supersedes
	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := NewLogStorage(DB)

	tx := beginLogTx(s, logID, t)
	defer tx.Close()
	root := trillian.SignedLogRoot{
		LogId:          logID,
		TimestampNanos: 98765,
		TreeSize:       16,
		TreeRevision:   5,
		RootHash:       []byte(dummyHash),
		Signature:      &spb.DigitallySigned{Signature: []byte("notempty")},
	}
	if err := tx.StoreSignedLogRoot(root); err != nil {
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
	if err := tx.StoreSignedLogRoot(root2); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}
	commit(tx, t)

	tx2 := beginLogTx(s, logID, t)
	defer tx2.Close()
	root3, err := tx2.LatestSignedLogRoot()
	if err != nil {
		t.Fatalf("Failed to read back new log root: %v", err)
	}
	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
	}
	commit(tx2, t)
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
	s := NewLogStorage(DB)

	logIDs, err := test.fn(s, context.Background(), logID)
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
	cleanTestDB(DB)
	logID1 := createLogForTests(DB)
	logID2 := createLogForTests(DB)
	logID3 := createLogForTests(DB)
	wantIds := []int64{logID1, logID2, logID3}
	runTestGetActiveLogIDsInternal(t, test, logID1, wantIds)
}

func runTestGetActiveLogIDsWithPendingWork(t *testing.T, test getActiveIDsTest) {
	cleanTestDB(DB)
	logID1 := createLogForTests(DB)
	logID2 := createLogForTests(DB)
	logID3 := createLogForTests(DB)
	s := NewLogStorage(DB)

	// Do a first run without any pending logs
	runTestGetActiveLogIDsInternal(t, test, logID1, nil)

	for _, logID := range []int64{logID1, logID2, logID3} {
		tx := beginLogTx(s, logID, t)
		defer tx.Close()
		leaves := createTestLeaves(leavesToInsert, 2)
		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves for log %v: %v", logID, err)
		}
		commit(tx, t)
	}

	wantIds := []int64{logID1, logID2, logID3}
	runTestGetActiveLogIDsInternal(t, test, logID1, wantIds)
}

func TestGetActiveLogIDs(t *testing.T) {
	getActiveIDsBegin := func(s storage.LogStorage, ctx context.Context, logID int64) ([]int64, error) {
		tx, err := s.BeginForTree(ctx, logID)
		if err != nil {
			return nil, err
		}
		defer tx.Close()
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
		defer tx.Close()
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
	s := NewLogStorage(DB)

	tx, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()

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
		defer tx.Close()
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
		defer tx.Close()
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
	s := NewLogStorage(DB)
	tx, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()
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
	cleanTestDB(DB)
	logID1 := createLogForTests(DB)
	logID2 := createLogForTests(DB)
	s := NewLogStorage(DB)

	{
		// Create fake leaf as if it had been sequenced
		data := []byte("some data")
		createFakeLeaf(DB, logID1, dummyHash, dummyRawHash, data, someExtraData, sequenceNumber, t)

		// Create fake leaves for second tree as if they had been sequenced
		data2 := []byte("some data 2")
		data3 := []byte("some data 3")
		createFakeLeaf(DB, logID2, dummyHash2, dummyRawHash, data2, someExtraData, sequenceNumber, t)
		createFakeLeaf(DB, logID2, dummyHash3, dummyRawHash, data3, someExtraData, sequenceNumber+1, t)
	}

	// Read back the leaf counts from both trees
	tx := beginLogTx(s, logID1, t)
	defer tx.Close()
	count1, err := tx.GetSequencedLeafCount()
	if err != nil {
		t.Fatalf("unexpected error getting leaf count: %v", err)
	}
	if want, got := int64(1), count1; want != got {
		t.Fatalf("expected %d sequenced for logId but got %d", want, got)
	}
	commit(tx, t)

	tx = beginLogTx(s, logID2, t)
	defer tx.Close()
	count2, err := tx.GetSequencedLeafCount()
	if err != nil {
		t.Fatalf("unexpected error getting leaf count2: %v", err)
	}
	if want, got := int64(2), count2; want != got {
		t.Fatalf("expected %d sequenced for logId2 but got %d", want, got)
	}
	commit(tx, t)
}

func TestSortByLeafIdentityHash(t *testing.T) {
	l := make([]*trillian.LogLeaf, 30)
	for i := range l {
		hash := sha256.Sum256([]byte{byte(i)})
		leaf := trillian.LogLeaf{
			LeafIdentityHash: hash[:],
			LeafValue:        []byte(fmt.Sprintf("Value %d", i)),
			ExtraData:        []byte(fmt.Sprintf("Extra %d", i)),
			LeafIndex:        int64(i),
		}
		l[i] = &leaf
	}
	sort.Sort(byLeafIdentityHash(l))
	for i := range l {
		if i == 0 {
			continue
		}
		if bytes.Compare(l[i-1].LeafIdentityHash, l[i].LeafIdentityHash) != -1 {
			t.Errorf("sorted leaves not in order, [%d] = %x, [%d] = %x", i-1, l[i-1].LeafIdentityHash, i, l[i].LeafIdentityHash)
		}
	}

}

func ensureAllLeavesDistinct(leaves []*trillian.LogLeaf, t *testing.T) {
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

// Convenience methods to avoid copying out "if err != nil { blah }" all over the place
func beginLogTx(s storage.LogStorage, logID int64, t *testing.T) storage.LogTreeTX {
	tx, err := s.BeginForTree(context.Background(), logID)
	if err != nil {
		t.Fatalf("Failed to begin log tx: %v", err)
	}
	return tx
}

type committableTX interface {
	Commit() error
}

func commit(tx committableTX, t *testing.T) {
	if err := tx.Commit(); err != nil {
		t.Errorf("Failed to commit tx: %v", err)
	}
}

func leafInBatch(leaf *trillian.LogLeaf, batch []*trillian.LogLeaf) bool {
	for _, bl := range batch {
		if bytes.Equal(bl.LeafIdentityHash, leaf.LeafIdentityHash) {
			return true
		}
	}

	return false
}

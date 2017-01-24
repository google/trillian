package mysql

import (
	"bytes"
	"database/sql"
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
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

var signedTimestamp = trillian.SignedEntryTimestamp{
	TimestampNanos: 1234567890,
	LogId:          createLogID("sign").logID,
	Signature:      &trillian.DigitallySigned{Signature: []byte("notempty")},
}

func createFakeLeaf(db *sql.DB, logID int64, rawHash, hash, data, extraData []byte, seq int64, t *testing.T) {
	_, err := db.Exec("INSERT INTO LeafData(TreeId, LeafValueHash, LeafValue, ExtraData) VALUES(?,?,?,?)", logID, rawHash, data, extraData)
	_, err2 := db.Exec("INSERT INTO SequencedLeafData(TreeId, SequenceNumber, LeafValueHash, MerkleLeafHash) VALUES(?,?,?,?)", logID, seq, rawHash, hash)

	if err != nil || err2 != nil {
		t.Fatalf("Failed to create test leaves: %v %v", err, err2)
	}
}

func checkLeafContents(leaf trillian.LogLeaf, seq int64, rawHash, hash, data, extraData []byte, t *testing.T) {
	if got, want := leaf.MerkleLeafHash, hash; !bytes.Equal(got, want) {
		t.Fatalf("Wrong leaf hash in returned leaf got\n%v\nwant:\n%v", got, want)
	}

	if got, want := leaf.LeafValueHash, rawHash; !bytes.Equal(got, want) {
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

func TestOpenStateCommit(t *testing.T) {
	logID := createLogID("TestOpenStateCommit")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx, err := s.Begin()

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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx, err := s.Begin()

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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer failIfTXStillOpen(t, "TestQueueLeaves", tx)

	leaves := createTestLeaves(leavesToInsert, 20)

	if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the
	// unsequenced data.
	var count int

	if err := db.QueryRow("SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID.logID).Scan(&count); err != nil {
		t.Fatalf("Could not query row count: %v", err)
	}

	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}

	// Additional check on timestamp being set correctly in the database
	var queueTimestamp int64
	if err := db.QueryRow("SELECT DISTINCT QueueTimestampNanos FROM Unsequenced WHERE TreeID=?", logID.logID).Scan(&queueTimestamp); err != nil {
		t.Fatalf("Could not query timestamp: %v", err)
	}

	if got, want := queueTimestamp, fakeQueueTime.UnixNano(); got != want {
		t.Fatalf("Incorrect queue timestamp got: %d want: %d", got, want)
	}
}

func TestQueueLeavesBadHash(t *testing.T) {
	logID := createLogID("TestQueueLeavesBadHash")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer failIfTXStillOpen(t, "TestQueueLeavesBadHash", tx)

	leaves := createTestLeaves(leavesToInsert, 20)

	// Deliberately corrupt one of the hashes so it should be rejected
	leaves[3].LeafValueHash = crypto.NewSHA256().Digest([]byte("this cannot be valid"))

	err := tx.QueueLeaves(leaves, fakeQueueTime)
	tx.Rollback()

	if err == nil {
		t.Fatal("Allowed a leaf to be queued with bad hash")
	}

	testonly.EnsureErrorContains(t, err, "mismatch")
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	logID := createLogID("TestDequeueLeavesNoneQueued")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	{
		tx := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestDequeLeaves", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue them
		tx2 := beginLogTx(s, t)
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
		tx3 := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	leavesToDequeue1 := 3
	leavesToDequeue2 := 2

	{
		tx := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeDequeueCutoffTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue some of them
		tx2 := beginLogTx(s, t)
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
		tx3 := beginLogTx(s, t)
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
		tx4 := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	{
		tx := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesGuardInterval", tx)

		leaves := createTestLeaves(leavesToInsert, 20)

		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue them using a cutoff that means we should get none
		tx2 := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	batchSize := 2

	{
		tx := beginLogTx(s, t)
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
		tx2 := beginLogTx(s, t)
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
		tx3 := beginLogTx(s, t)
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
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
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

func TestGetLeavesByLeafValueHashNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByLeafValueHashNotPresent")
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer commit(tx, t)

	hashes := [][]byte{[]byte("thisdoesn'texist")}
	if leaves, err := tx.GetLeavesByLeafValueHash(hashes, false); err != nil {
		t.Fatalf("GetLeavesByLeafValueHash(%x)=nil,%v; want [],nil", hashes, err)
	} else if len(leaves) != 0 {
		t.Fatalf("GetLeavesByLeafValueHash(%s)=%q,nil; want [],nil", hashes, leaves)
	}
}

func TestGetLeavesByIndexNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByIndexNotPresent")
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer commit(tx, t)

	_, err := tx.GetLeavesByIndex([]int64{99999})

	if err == nil {
		t.Fatalf("Returned ok for leaf index when nothing inserted: %v", err)
	}
}

func TestGetLeavesByHash(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByHash")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	data := []byte("some data")

	createFakeLeaf(db, logID.logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, t)
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

func TestGetLeavesByLeafValueHash(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByLeafValueHash")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	data := []byte("some data")

	createFakeLeaf(db, logID.logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, t)
	defer commit(tx, t)

	hashes := [][]byte{dummyRawHash}
	if leaves, err := tx.GetLeavesByLeafValueHash(hashes, false); err != nil {
		t.Fatalf("GetLeavesByLeafValueHash(%x)=nil,%v; want [],nil", hashes, err)
	} else if len(leaves) != 1 {
		t.Fatalf("GetLeavesByLeafValueHash() = %d leaves, want 1", len(leaves))
	} else {
		checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, someExtraData, t)
	}
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced, read it back and check contents
	logID := createLogID("TestGetLeavesByIndex")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	data := []byte("some data")

	createFakeLeaf(db, logID.logID, dummyRawHash, dummyHash, data, someExtraData, sequenceNumber, t)

	tx := beginLogTx(s, t)
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

func openTestDBOrDie() *sql.DB {
	db, err := sql.Open("mysql", "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		panic(err)
	}

	return db
}

func TestLatestSignedRootNoneWritten(t *testing.T) {
	logID := createLogID("TestLatestSignedLogRootNoneWritten")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Rollback()

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	commit(tx, t)

	{
		tx2 := beginLogTx(s, t)
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

func TestGetTreeRevisionAtSize(t *testing.T) {
	logID := createLogID("TestGetTreeRevisionAtSize")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	{
		tx := beginLogTx(s, t)

		// TODO: Tidy up the log id as it looks silly chained 3 times like this
		root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}
		root2 := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 198765, TreeSize: 27, TreeRevision: 11, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

		if err := tx.StoreSignedLogRoot(root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}

		if err := tx.StoreSignedLogRoot(root2); err != nil {
			t.Fatalf("Failed to store signed root2: %v", err)
		}

		commit(tx, t)
	}

	{
		tx := beginLogTx(s, t)
		defer commit(tx, t)

		// First two are tree head sizes and returned version should match
		if treeRevision1, treeSize1, err := tx.GetTreeRevisionIncludingSize(16); err != nil || treeRevision1 != 5 || treeSize1 != 16 {
			t.Fatalf("Want revision=5, size=16, err=nil got: revision=%d size=%d err=%v", treeRevision1, treeSize1, err)
		}

		if treeRevision2, treeSize2, err := tx.GetTreeRevisionIncludingSize(27); err != nil || treeRevision2 != 11 || treeSize2 != 27 {
			t.Fatalf("Want revision=11, size=27, err=nil got: revision=%d size=%d err=%v", treeRevision2, treeSize2, err)
		}

		// A tree size between revisions should return the next highest
		if treeRevision3, treeSize3, err := tx.GetTreeRevisionIncludingSize(21); err != nil || treeRevision3 != 11 || treeSize3 != 27 {
			t.Fatalf("Want revision=11, size=27, err=nil got: revision=%d size=%d err=%v", treeRevision3, treeSize3, err)
		}

		// A value >= largest tree size should not be allowed
		if treeRevision4, treeSize4, err := tx.GetTreeRevisionIncludingSize(30); err == nil {
			t.Fatalf("Got: revision=%d size=%d err=%v for tree size 30, want: 0, 0, error", treeRevision4, treeSize4, err)
		}
	}
}

func TestGetTreeRevisionMultipleSameSize(t *testing.T) {
	logID := createLogID("TestGetTreeRevisionAtSize")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	{
		tx := beginLogTx(s, t)

		// Normally tree heads at the same tree size must have the same revision because nothing was
		// added between them by definition, this is an artificial situation just for testing.
		// TODO: Tidy up the log id as it looks silly chained 3 times like this
		root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 11, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}
		root2 := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 198765, TreeSize: 16, TreeRevision: 13, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

		if err := tx.StoreSignedLogRoot(root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}

		if err := tx.StoreSignedLogRoot(root2); err != nil {
			t.Fatalf("Failed to store signed root2: %v", err)
		}

		commit(tx, t)
	}

	{
		tx := beginLogTx(s, t)
		defer commit(tx, t)

		// We should get back the first revision at size 16
		treeRevision, treeSize, err := tx.GetTreeRevisionIncludingSize(16)

		if err != nil {
			t.Fatalf("Failed to get tree revision: %v", err)
		}

		if treeRevision != 11 || treeSize != 16 {
			t.Fatalf("got revision=%d, size=%d, want: revision=11, size=16", treeRevision, treeSize)
		}
	}
}

func TestDuplicateSignedLogRoot(t *testing.T) {
	logID := createLogID("TestDuplicateSignedLogRoot")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)

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

	tx2 := beginLogTx(s, t)
	defer commit(tx2, t)
	root3, err := tx2.LatestSignedLogRoot()

	if err != nil {
		t.Fatalf("Failed to read back new log root: %v", err)
	}

	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
	}
}

func TestGetActiveLogIDs(t *testing.T) {
	// Have to wipe everything to ensure we start with zero log trees configured
	cleanTestDB()
	logID := createLogID("TestGetActiveLogIDs")
	s := prepareTestLogStorage(logID, t)

	// This creates one tree
	db := prepareTestLogDB(logID, t)
	defer db.Close()

	tx := beginLogTx(s, t)

	logIDs, err := tx.GetActiveLogIDs()

	if err != nil {
		t.Fatalf("Failed to get log ids: %v", err)
	}

	if got, want := len(logIDs), 1; got != want {
		t.Fatalf("Got %d logID(s), wanted %d", got, want)
	}
}

func TestGetActiveLogIDsWithPendingWork(t *testing.T) {
	// Have to wipe everything to ensure we start with zero log trees configured
	cleanTestDB()
	logID := createLogID("TestGetActiveLogIDsWithPendingWork")
	s := prepareTestLogStorage(logID, t)
	db := prepareTestLogDB(logID, t)
	defer db.Close()

	tx := beginLogTx(s, t)

	logIDs, err := tx.GetActiveLogIDsWithPendingWork()
	commit(tx, t)

	if err != nil || len(logIDs) != 0 {
		t.Fatalf("Should have had no logs with unsequenced work but got: %v %v", logIDs, err)
	}

	{
		tx := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestGetActiveLogIDsFiltered", tx)

		leaves := createTestLeaves(leavesToInsert, 2)

		if err := tx.QueueLeaves(leaves, fakeQueueTime); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	// We should now see the logID that we just created work for
	tx = beginLogTx(s, t)

	logIDs, err = tx.GetActiveLogIDsWithPendingWork()
	commit(tx, t)

	if err != nil || len(logIDs) != 1 {
		t.Fatalf("Should have had one log with unsequenced work but got: %v", logIDs)
	}

	expected, got := logID.logID, logIDs[0]
	if expected != got {
		t.Fatalf("Expected to see tree ID: %d but got: %d", expected, got)
	}
}

func TestGetSequencedLeafCount(t *testing.T) {
	// We'll create leaves for two different trees
	logID := createLogID("TestGetSequencedLeafCount")
	logID2 := createLogID("TestGetSequencedLeafCount2")
	s1 := prepareTestLogStorage(logID, t)
	s2 := prepareTestLogStorage(logID2, t)
	{
		db := prepareTestLogDB(logID, t)

		// Create fake leaf as if it had been sequenced
		defer db.Close()

		data := []byte("some data")

		createFakeLeaf(db, logID.logID, dummyHash, dummyRawHash, data, someExtraData, sequenceNumber, t)

		// Create fake leaves for second tree as if they had been sequenced
		db2 := prepareTestLogDB(logID2, t)
		defer db2.Close()

		data2 := []byte("some data 2")
		data3 := []byte("some data 3")

		createFakeLeaf(db2, logID2.logID, dummyHash2, dummyRawHash, data2, someExtraData, sequenceNumber, t)
		createFakeLeaf(db2, logID2.logID, dummyHash3, dummyRawHash, data3, someExtraData, sequenceNumber+1, t)
	}

	// Read back the leaf counts from both trees
	tx := beginLogTx(s1, t)
	count1, err := tx.GetSequencedLeafCount()
	commit(tx, t)

	if err != nil {
		t.Fatalf("unexpected error getting leaf count: %v", err)
	}

	if want, got := int64(1), count1; want != got {
		t.Fatalf("expected %d sequenced for logId but got %d", want, got)
	}

	tx = beginLogTx(s2, t)
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
			if i != j && bytes.Equal(leaves[i].LeafValueHash, leaves[j].LeafValueHash) {
				t.Fatalf("Unexpectedly got a duplicate leaf hash: %v %v",
					leaves[i].LeafValueHash, leaves[j].LeafValueHash)
			}
		}
	}
}

func prepareTestLogStorage(logID logIDAndTest, t *testing.T) storage.LogStorage {
	if err := DeleteLog(logID.logID, "test:zaphod@tcp(127.0.0.1:3306)/test"); err != nil {
		t.Fatalf("Failed to delete log storage: %s", err)
	}
	if err := CreateLog(logID.logID, "test:zaphod@tcp(127.0.0.1:3306)/test"); err != nil {
		t.Fatalf("Failed to create new log storage: %s", err)
	}
	s, err := NewLogStorage(logID.logID, "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		t.Fatalf("Failed to open log storage: %s", err)
	}
	return s
}

// This removes all database contents for the specified log id so tests run in a
// predictable environment. For obvious reasons this should only be allowed to run
// against test databases. This method panics if any of the deletions fails to make
// sure tests can't inadvertently succeed.
func prepareTestTreeDB(treeID int64, t *testing.T) *sql.DB {
	db := openTestDBOrDie()

	// Wipe out anything that was there for this tree id
	for _, table := range allTables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE TreeId=?", table), treeID)

		if err != nil {
			t.Fatalf("Failed to delete rows in %s for %d: %s", table, treeID, err)
		}
	}
	return db
}

// This removes all database contents for the specified log id so tests run in a
// predictable environment. For obvious reasons this should only be allowed to run
// against test databases. This method panics if any of the deletions fails to make
// sure tests can't inadvertently succeed.
func prepareTestLogDB(logID logIDAndTest, t *testing.T) *sql.DB {
	db := prepareTestTreeDB(logID.logID, t)

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, logID.logID, logID.logID)

	if err != nil {
		t.Fatalf("Failed to create tree entry for test: %v", err)
	}

	return db
}

// This deletes all the entries in the database. Only use this with a test database
func cleanTestDB() {
	db := openTestDBOrDie()
	defer db.Close()

	// Wipe out anything that was there for this log id
	for _, table := range allTables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))

		if err != nil {
			panic(fmt.Errorf("Failed to delete rows in %s: %s", table, err))
		}
	}
}

// Creates some test leaves with predictable data
func createTestLeaves(n, startSeq int64) []trillian.LogLeaf {
	var leaves []trillian.LogLeaf
	hasher := crypto.NewSHA256()

	for l := int64(0); l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l+startSeq)
		leaf := trillian.LogLeaf{
			LeafValueHash:  hasher.Digest([]byte(lv)),
			MerkleLeafHash: hasher.Digest([]byte(lv)),
			LeafValue:      []byte(lv),
			ExtraData:      []byte(fmt.Sprintf("Extra %d", l)),
			LeafIndex:      int64(startSeq + l),
		}
		leaves = append(leaves, leaf)
	}

	return leaves
}

// Convenience methods to avoid copying out "if err != nil { blah }" all over the place
func commit(tx storage.LogTX, t *testing.T) {
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit tx: %v", err)
	}
}

func beginLogTx(s storage.LogStorage, t *testing.T) storage.LogTX {
	tx, err := s.Begin()

	if err != nil {
		t.Fatalf("Failed to begin log tx: %v", err)
	}

	return tx
}

func failIfTXStillOpen(t *testing.T, op string, tx storage.LogTX) {
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

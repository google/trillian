package mysql

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"time"
)

// TODO(al): add checking to all the Commit() calls in here.

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

const leavesToInsert = 5
const sequenceNumber int64 = 237

type logIDAndTest struct {
	logID    int64
	testName string
}

type mapIDAndTest struct {
	mapID    int64
	testName string
}

// Tests that access the db should each use a distinct log ID to prevent lock contention when
// run in parallel or race conditions / unexpected interactions. Tests that pass should hold
// no locks afterwards.

var signedTimestamp = trillian.SignedEntryTimestamp{
	TimestampNanos: 1234567890, LogId: createLogID("sign").logID, Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

// Parallel tests must get different log or map ids
var idMutex sync.Mutex
var testLogID int64
var testMapID int64

func createSomeNodes(testName string, treeID int64) []storage.Node {
	r := make([]storage.Node, 4)
	for i := range r {
		r[i].NodeID = storage.NewNodeIDWithPrefix(uint64(i), 8, 8, 8)
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		glog.Infof("Node to store: %v\n", r[i].NodeID)
	}
	return r
}

func createLogID(testName string) logIDAndTest {
	idMutex.Lock()
	defer idMutex.Unlock()
	testLogID++

	return logIDAndTest{logID: testLogID, testName: testName}
}

func createMapID(testName string) mapIDAndTest {
	idMutex.Lock()
	defer idMutex.Unlock()
	testLogID++

	return mapIDAndTest{mapID: testMapID, testName: testName}
}

func nodesAreEqual(lhs []storage.Node, rhs []storage.Node) error {
	if ls, rs := len(lhs), len(rhs); ls != rs {
		return fmt.Errorf("different number of nodes, %d vs %d", ls, rs)
	}
	for i := range lhs {
		if l, r := lhs[i].NodeID.String(), rhs[i].NodeID.String(); l != r {
			return fmt.Errorf("NodeIDs are not the same,\nlhs = %v,\nrhs = %v", l, r)
		}
		if l, r := lhs[i].Hash, rhs[i].Hash; !bytes.Equal(l, r) {
			return fmt.Errorf("Hashes are not the same,\nlhs = %v,\nrhs = %v", l, r)
		}
	}
	return nil
}

func createFakeLeaf(db *sql.DB, logID int64, rawHash, hash []byte, data []byte, seq int64, t *testing.T) {
	_, err := db.Exec("INSERT INTO LeafData(TreeId, LeafValueHash, LeafValue) VALUES(?,?,?)", logID, rawHash, data)
	_, err2 := db.Exec("INSERT INTO SequencedLeafData(TreeId, SequenceNumber, LeafValueHash, MerkleLeafHash) VALUES(?,?,?,?)", logID, seq, rawHash, hash)

	if err != nil || err2 != nil {
		t.Fatalf("Failed to create test leaves: %v %v", err, err2)
	}
}

func checkLeafContents(leaf trillian.LogLeaf, seq int64, rawHash, hash, data []byte, t *testing.T) {
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
}

func TestOpenStateCommit(t *testing.T) {
	logID := createLogID("TestOpenStateCommit")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx, err := s.Begin()

	if err != nil {
		t.Fatalf("Failed to set up db transaction")
	}

	if !tx.IsOpen() {
		t.Fatalf("Transaction should be open on creation")
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if tx.IsOpen() {
		t.Fatalf("Transaction should be closed after commit")
	}
}

func TestOpenStateRollback(t *testing.T) {
	logID := createLogID("TestOpenStateRollback")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx, err := s.Begin()

	if err != nil {
		t.Fatalf("Failed to set up db transaction")
	}

	if !tx.IsOpen() {
		t.Fatalf("Transaction should be open on creation")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if tx.IsOpen() {
		t.Fatalf("Transaction should be closed after rollback")
	}
}

func forceWriteRevision(rev int64, tx storage.TreeTX) {
	mtx, ok := tx.(*logTX)
	if !ok {
		panic(nil)
	}
	mtx.treeTX.writeRevision = rev
}

func TestNodeRoundTrip(t *testing.T) {
	logID := createLogID("TestNodeRoundTrip")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	const writeRevision = int64(100)

	nodesToStore := createSomeNodes("TestNodeRoundTrip", logID.logID)
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	{
		tx, err := s.Begin()
		forceWriteRevision(writeRevision, tx)
		if err != nil {
			t.Fatalf("Failed to Begin: %s", err)
		}

		// Need to read nodes before attempting to write
		if _, err := tx.GetMerkleNodes(99, nodeIDsToRead); err != nil {
			t.Fatalf("Failed to read nodes: %s", err)
		}

		if err := tx.SetMerkleNodes(nodesToStore); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit nodes: %s", err)
		}
	}

	{
		tx, err := s.Begin()

		if err != nil {
			t.Fatalf("Failed to Begin: %s", err)
		}

		readNodes, err := tx.GetMerkleNodes(100, nodeIDsToRead)
		if err != nil {
			t.Fatalf("Failed to retrieve nodes: %s", err)
		}
		if err := nodesAreEqual(readNodes, nodesToStore); err != nil {
			t.Fatalf("Read back different nodes from the ones stored: %s", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit read: %s", err)
		}
	}
}

// Explicit test for node id conversion to / from protos.
func TestNodeIDSerialization(t *testing.T) {
	nodeID := storage.NodeID{Path: []byte("hello"), PrefixLenBits: 3, PathLenBits: 40}
	serializedBytes, err := encodeNodeID(nodeID)

	if err != nil {
		t.Fatalf("Failed to serialize NodeID: %v, %v", nodeID, err)
	}

	nodeID2, err := decodeNodeID(serializedBytes)

	if err != nil {
		t.Fatalf("Failed to deserialize NodeID: %v, %v", nodeID, err)
	}

	t.Logf("1:\n%v (%s)\n2:\n%v (%s)", nodeID, nodeID.String(), *nodeID2, nodeID2.String())

	if expected, got := nodeID.String(), nodeID2.String(); expected != got {
		t.Errorf("Round trip of nodeID failed: %v %v", expected, got)
	}
}

func TestQueueDuplicateLeafFails(t *testing.T) {
	logID := createLogID("TestQueueDuplicateLeafFails")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

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
		t.Fatalf("Could not query row count")
	}

	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}

	// Additional check on timestamp being set correctly in the database
	var queueTimestamp int64
	if err := db.QueryRow("SELECT DISTINCT QueueTimestampNanos FROM Unsequenced WHERE TreeID=?", logID.logID).Scan(&queueTimestamp); err != nil {
		t.Fatalf("Could not query timestamp")
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
	leaves[3].MerkleLeafHash = crypto.NewSHA256().Digest([]byte("this cannot be valid"))

	err := tx.QueueLeaves(leaves, fakeQueueTime)
	tx.Rollback()

	if err == nil {
		t.Fatalf("Allowed a leaf to be queued with bad hash")
	}

	testonly.EnsureErrorContains(t, err, "mismatch")
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	logID := createLogID("TestDequeueLeavesNoneQueued")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

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

		ensureAllLeafHashesDistinct(leaves2, t)

		tx2.Commit()
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

		ensureAllLeafHashesDistinct(leaves2, t)

		tx2.Commit()

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

		ensureAllLeafHashesDistinct(leaves3, t)

		// Plus the union of the leaf batches should all have distinct hashes
		leaves4 := append(leaves2, leaves3...)
		ensureAllLeafHashesDistinct(leaves4, t)

		tx3.Commit()
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

		tx4.Commit()
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

		ensureAllLeafHashesDistinct(leaves2, t)

		tx2.Commit()
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByHashNotPresent")
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

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
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

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

	data := []byte("some data")

	createFakeLeaf(db, logID.logID, dummyRawHash, dummyHash, data, sequenceNumber, t)

	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	hashes := [][]byte{dummyHash}
	leaves, err := tx.GetLeavesByHash(hashes, false)

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by hash: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, t)
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByIndex")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	data := []byte("some data")

	createFakeLeaf(db, logID.logID, dummyRawHash, dummyHash, data, sequenceNumber, t)

	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	leaves, err := tx.GetLeavesByIndex([]int64{sequenceNumber})

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by index: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyRawHash, dummyHash, data, t)
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

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new log root: %v", err)
	}

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

func TestGetTreeRevisionAtNonExistentSizeError(t *testing.T) {
	// Have to set all this up though we won't actually write anything
	logID := createLogID("TestGetTreeRevisionAtSize")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	if _, err := tx.GetTreeRevisionAtSize(0); err == nil {
		t.Fatalf("Returned a tree revision for 0 sized tree")
	}

	if _, err := tx.GetTreeRevisionAtSize(-427); err == nil {
		t.Fatalf("Returned a tree revision for -ve sized tree")
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

		tx.Commit()
	}

	{
		tx := beginLogTx(s, t)
		defer tx.Commit()

		// First two are legit tree head sizes and should work
		treeRevision1, err := tx.GetTreeRevisionAtSize(16)

		if err != nil {
			t.Fatalf("Failed to get tree revision1: %v", err)
		}

		treeRevision2, err := tx.GetTreeRevisionAtSize(27)

		if err != nil {
			t.Fatalf("Failed to get tree revision2: %v", err)
		}

		if treeRevision1 != 5 || treeRevision2 != 11 {
			t.Fatalf("Expected tree revisions 5,11 but got %d,%d", treeRevision1, treeRevision2)
		}

		// But an intermediate value shouldn't work
		treeRevision3, err := tx.GetTreeRevisionAtSize(21)

		if err == nil {
			t.Fatalf("Unexpectedly returned revision for nonexistent tree size: %d", treeRevision3)
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

		tx.Commit()
	}

	{
		tx := beginLogTx(s, t)
		defer tx.Commit()

		// We should get back the highest revision at size 16
		treeRevision, err := tx.GetTreeRevisionAtSize(16)

		if err != nil {
			t.Fatalf("Failed to get tree revision: %v", err)
		}

		if treeRevision != 13 {
			t.Fatalf("Expected tree revisions 13 but got %d", treeRevision)
		}
	}
}

func TestDuplicateSignedLogRoot(t *testing.T) {
	logID := createLogID("TestDuplicateSignedLogRoot")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root := trillian.SignedLogRoot{LogId: logID.logID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	// Shouldn't be able to do it again
	if err := tx.StoreSignedLogRoot(root); err == nil {
		t.Fatalf("Allowed duplicate signed root")
	}
}

func TestLogRootUpdate(t *testing.T) {
	// Write two roots for a log and make sure the one with the newest timestamp supersedes
	logID := createLogID("TestLatestSignedLogRoot")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

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

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new log roots: %v", err)
	}

	tx = beginLogTx(s, t)
	root3, err := tx.LatestSignedLogRoot()

	if err != nil {
		t.Fatalf("Failed to read back new log root: %v", err)
	}

	if !proto.Equal(&root2, &root3) {
		t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
	}
}

// ---- MapStorage tests below:

func TestLatestSignedMapRootNoneWritten(t *testing.T) {
	mapID := createMapID("TestLatestSignedMapRootNoneWritten")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)
	tx := beginMapTx(s, t)
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
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)
	tx := beginMapTx(s, t)
	defer tx.Rollback()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new map root: %v", err)
	}

	{
		tx2 := beginMapTx(s, t)
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
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)
	tx := beginMapTx(s, t)
	defer tx.Commit()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	// Shouldn't be able to do it again
	if err := tx.StoreSignedMapRoot(root); err == nil {
		t.Fatalf("Allowed duplicate signed map root")
	}
}

func TestMapRootUpdate(t *testing.T) {
	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	mapID := createMapID("TestLatestSignedMapRoot")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)
	tx := beginMapTx(s, t)
	defer tx.Commit()

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root2 := trillian.SignedMapRoot{MapId: mapID.mapID, TimestampNanos: 98766, MapRevision: 6, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root2); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit new map roots: %v", err)
	}

	tx = beginMapTx(s, t)
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
	KeyHash:   keyHash,
	LeafHash:  []byte("A Hash"),
	LeafValue: []byte("A Value"),
	ExtraData: []byte("Some Extra Data"),
}

func TestMapSetGetRoundTrip(t *testing.T) {
	cleanTestDB()

	mapID := createMapID("TestMapSetGetRoundTrip")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)

	readRev := int64(1)

	{
		tx := beginMapTx(s, t)

		if err := tx.Set(keyHash, mapLeaf); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	{
		tx := beginMapTx(s, t)

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
	cleanTestDB()

	mapID := createMapID("TestMapSetSameKeyInSameRevisionFails")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)

	{
		tx := beginMapTx(s, t)

		if err := tx.Set(keyHash, mapLeaf); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	{
		tx := beginMapTx(s, t)

		if err := tx.Set(keyHash, mapLeaf); err == nil {
			t.Fatalf("Unexpectedly succeeded in setting %v to %v", keyHash, mapLeaf)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}
}

func TestMapGetUnknownKey(t *testing.T) {
	cleanTestDB()

	mapID := createMapID("TestMapGetUnknownKey")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)

	{
		tx := beginMapTx(s, t)

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
	cleanTestDB()

	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	mapID := createMapID("TestMapSetGetMultipleRevisions")
	db := prepareTestMapDB(mapID, t)
	defer db.Close()
	s := prepareTestMapStorage(mapID, t)

	numRevs := 3
	values := make([]trillian.MapLeaf, numRevs)
	for i := 0; i < numRevs; i++ {
		values[i] = trillian.MapLeaf{
			KeyHash:   keyHash,
			LeafHash:  []byte(fmt.Sprintf("A Hash %d", i)),
			LeafValue: []byte(fmt.Sprintf("A Value %d", i)),
			ExtraData: []byte(fmt.Sprintf("Some Extra Data %d", i)),
		}
	}

	for i := 0; i < numRevs; i++ {
		tx := beginMapTx(s, t)
		mysqlMapTX := tx.(*mapTX)
		mysqlMapTX.treeTX.writeRevision = int64(i)
		if err := tx.Set(keyHash, values[i]); err != nil {
			t.Fatalf("Failed to set %v to %v: %v", keyHash, values[i], err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	for i := 0; i < numRevs; i++ {
		tx := beginMapTx(s, t)

		readValues, err := tx.Get(int64(i), [][]byte{keyHash})
		if err != nil {
			t.Fatalf("At rev %d failed to get %v:  %v", i, keyHash, err)
		}
		if got, want := len(readValues), 1; got != want {
			t.Fatalf("At rev %d got %d values, expected %d", i, got, want)
		}
		if got, want := &readValues[0], &values[i]; !proto.Equal(got, want) {
			t.Fatalf("At rev %d read back %v, but expected %v", i, got, want)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("At rev %d failed to commit: %v", i, err)
		}
	}
}

func TestGetActiveLogIDs(t *testing.T) {
	// Have to wipe everything to ensure we start with zero log trees configured
	cleanTestDB()

	// This creates one tree
	logID := createLogID("TestGetActiveLogIDs")
	db := prepareTestLogDB(logID, t)
	defer db.Close()

	s := prepareTestLogStorage(logID, t)
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
	db := prepareTestLogDB(logID, t)
	defer db.Close()

	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)

	logIDs, err := tx.GetActiveLogIDsWithPendingWork()
	tx.Commit()

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
	tx.Commit()

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

	{
		db := prepareTestLogDB(logID, t)

		// Create fake leaf as if it had been sequenced
		defer db.Close()

		data := []byte("some data")

		createFakeLeaf(db, logID.logID, dummyHash, dummyRawHash, data, sequenceNumber, t)

		// Create fake leaves for second tree as if they had been sequenced
		db2 := prepareTestLogDB(logID2, t)
		defer db2.Close()

		data2 := []byte("some data 2")
		data3 := []byte("some data 3")

		createFakeLeaf(db2, logID2.logID, dummyHash2, dummyRawHash, data2, sequenceNumber, t)
		createFakeLeaf(db2, logID2.logID, dummyHash3, dummyRawHash, data3, sequenceNumber+1, t)
	}

	// Read back the leaf counts from both trees
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	count1, err := tx.GetSequencedLeafCount()
	tx.Commit()

	if err != nil {
		t.Fatalf("unexpected error getting leaf count: %v", err)
	}

	if want, got := int64(1), count1; want != got {
		t.Fatalf("expected %d sequenced for logId but got %d", want, got)
	}

	s = prepareTestLogStorage(logID2, t)
	tx = beginLogTx(s, t)
	count2, err := tx.GetSequencedLeafCount()
	tx.Commit()

	if err != nil {
		t.Fatalf("unexpected error getting leaf count2: %v", err)
	}

	if want, got := int64(2), count2; want != got {
		t.Fatalf("expected %d sequenced for logId2 but got %d", want, got)
	}
}

func ensureAllLeafHashesDistinct(leaves []trillian.LogLeaf, t *testing.T) {
	// All the hashes should be distinct. If only we had maps with slices as keys or sets
	// or pretty much any kind of usable data structures we could do this properly.
	for i := range leaves {
		for j := range leaves {
			if i != j && bytes.Equal(leaves[i].MerkleLeafHash, leaves[j].MerkleLeafHash) {
				t.Fatalf("Unexpectedly got a duplicate leaf hash: %v %v",
					leaves[i].MerkleLeafHash, leaves[j].MerkleLeafHash)
			}
		}
	}
}

func createTestDB() {
	db := openTestDBOrDie()
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(23, "hi", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
	}
}

func prepareTestLogStorage(logID logIDAndTest, t *testing.T) storage.LogStorage {
	s, err := NewLogStorage(logID.logID, "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		t.Fatalf("Failed to open log storage: %s", err)
	}

	return s
}

func prepareTestMapStorage(mapID mapIDAndTest, t *testing.T) storage.MapStorage {
	s, err := NewMapStorage(mapID.mapID, "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		t.Fatalf("Failed to open map storage: %s", err)
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

// This removes all database contents for the specified map id so tests run in a
// predictable environment. For obvious reasons this should only be allowed to run
// against test databases. This method panics if any of the deletions fails to make
// sure tests can't inadvertently succeed.
func prepareTestMapDB(mapID mapIDAndTest, t *testing.T) *sql.DB {
	db := prepareTestTreeDB(mapID.mapID, t)

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, mapID.mapID, mapID.mapID)

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
		lv := fmt.Sprintf("Leaf %d", l)
		leaf := trillian.LogLeaf{
			MerkleLeafHash: hasher.Digest([]byte(lv)), LeafValue: []byte(lv), ExtraData: []byte(fmt.Sprintf("Extra %d", l)), LeafIndex: int64(startSeq + l)}
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

func beginMapTx(s storage.MapStorage, t *testing.T) storage.MapTX {
	tx, err := s.Begin()

	if err != nil {
		t.Fatalf("Failed to begin map tx: %v", err)
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

func TestMain(m *testing.M) {
	flag.Parse()
	cleanTestDB()
	createTestDB()
	os.Exit(m.Run())
}

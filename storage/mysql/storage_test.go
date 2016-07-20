package mysql

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/assert"
)

var allTables = []string{"Unsequenced", "TreeHead", "SequencedLeafData", "LeafData", "Subtree", "TreeControl", "Trees", "MapLeaf", "MapHead"}

// Must be 32 bytes to match sha256 length if it was a real hash
var dummyHash = []byte("hashxxxxhashxxxxhashxxxxhashxxxx")

const leavesToInsert = 5
const sequenceNumber int64 = 237

type logIDAndTest struct {
	logID    trillian.LogID
	testName string
}

type mapIDAndTest struct {
	mapID    trillian.MapID
	testName string
}

// Tests that access the db should each use a distinct log ID to prevent lock contention when
// run in parallel or race conditions / unexpected interactions. Tests that pass should hold
// no locks afterwards.

var signedTimestamp = trillian.SignedEntryTimestamp{
	TimestampNanos: 1234567890, LogId: createLogID("sign").logID.LogID, Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

// Parallel tests must get different log or map ids
var idMutex sync.Mutex
var testLogId int64
var testMapId int64

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
	testLogId++

	return logIDAndTest{logID: trillian.LogID{LogID: []byte(testName), TreeID: testLogId}, testName: testName}
}

func createMapID(testName string) mapIDAndTest {
	idMutex.Lock()
	defer idMutex.Unlock()
	testLogId++

	return mapIDAndTest{mapID: trillian.MapID{MapID: []byte(testName), TreeID: testMapId}, testName: testName}
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

func createFakeLeaf(db *sql.DB, logID trillian.LogID, hash, data, tsBytes []byte, seq int64, t *testing.T) {
	_, err := db.Exec("INSERT INTO LeafData(TreeId, LeafHash, TheData) VALUES(?,?,?)", logID.TreeID, hash, data)
	_, err2 := db.Exec("INSERT INTO SequencedLeafData(TreeId, SequenceNumber, LeafHash, SignedEntryTimestamp) VALUES(?,?,?,?)", logID.TreeID, seq, hash, tsBytes)

	if err != nil || err2 != nil {
		t.Fatalf("Failed to create test leaves: %v %v", err, err2)
	}
}

func checkLeafContents(leaf trillian.LogLeaf, seq int64, hash, data []byte, t *testing.T) {
	if expected, got := hash, leaf.LeafHash; !bytes.Equal(expected, got) {
		t.Fatalf("Unexpected leaf hash in returned leaf. Expected:\n%v\nGot:\n%v", expected, got)
	}

	if leaf.SequenceNumber != seq {
		t.Fatalf("Bad sequence number in returned leaf: %d", leaf.SequenceNumber)
	}

	if expected, got := data, leaf.LeafValue; !bytes.Equal(data, leaf.LeafValue) {
		t.Fatalf("Unxpected data in returned leaf. Expected:\n%v\nGot:\n%v", expected, got)
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

	assert.True(t, tx.IsOpen(), "Transaction should be open on creation")
	err = tx.Commit()
	assert.Nil(t, err, "Failed to commit: %v", err)
	assert.False(t, tx.IsOpen(), "Transaction should be closed after commit")
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

	assert.True(t, tx.IsOpen(), "Transaction should be open on creation")
	err = tx.Rollback()
	assert.Nil(t, err, "Failed to commit: %v", err)
	assert.False(t, tx.IsOpen(), "Transaction should be closed after rollback")
}

func TestNodeRoundTrip(t *testing.T) {
	logID := createLogID("TestNodeRoundTrip")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)

	nodesToStore := createSomeNodes("TestNodeRoundTrip", logID.logID.TreeID)

	{
		tx, err := s.Begin()
		if err != nil {
			t.Fatalf("Failed to Begin: %s", err)
		}

		if err := tx.SetMerkleNodes(100, nodesToStore); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit nodes: %s", err)
		}
	}

	{
		tx, err := s.Begin()
		defer tx.Commit()

		if err != nil {
			t.Fatalf("Failed to Begin: %s", err)
		}

		nodeIDs := make([]storage.NodeID, len(nodesToStore))
		for i := range nodesToStore {
			nodeIDs[i] = nodesToStore[i].NodeID
		}
		readNodes, err := tx.GetMerkleNodes(100, nodeIDs)
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
	nodeID := storage.NodeID{[]byte("hello"), 3, 40}
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

	if err := tx.QueueLeaves(leaves); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	leaves2 := createTestLeaves(5, 12)

	if err := tx.QueueLeaves(leaves2); err == nil {
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

	if err := tx.QueueLeaves(leaves); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the
	// unsequenced data.
	var count int

	if err := db.QueryRow("SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID.logID.TreeID).Scan(&count); err != nil {
		t.Fatalf("Could not query row count")
	}

	if leavesToInsert != count {
		t.Fatalf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}
}

func TestDequeueLeavesNoneQueued(t *testing.T) {
	logID := createLogID("TestDequeueLeavesNoneQueued")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	leaves, err := tx.DequeueLeaves(999)

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

		if err := tx.QueueLeaves(leaves); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue them
		tx2 := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestDequeLeaves", tx2)
		leaves2, err := tx2.DequeueLeaves(99)

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

		leaves3, err := tx3.DequeueLeaves(99)

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

		if err := tx.QueueLeaves(leaves); err != nil {
			t.Fatalf("Failed to queue leaves: %v", err)
		}

		commit(tx, t)
	}

	{
		// Now try to dequeue some of them
		tx2 := beginLogTx(s, t)
		defer failIfTXStillOpen(t, "TestDequeueLeavesTwoBatches-tx2", tx2)
		leaves2, err := tx2.DequeueLeaves(leavesToDequeue1)

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
		leaves3, err := tx3.DequeueLeaves(leavesToDequeue2)

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

		leaves5, err := tx4.DequeueLeaves(99)

		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}

		if len(leaves5) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves5))
		}

		tx4.Commit()
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByHashNotPresent")
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	hashes := []trillian.Hash{trillian.Hash("thisdoesn'texist")}
	leaves, err := tx.GetLeavesByHash(hashes)

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
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	data := []byte("some data")

	signedTimestampBytes, err := EncodeSignedTimestamp(signedTimestamp)

	if err != nil {
		t.Fatalf("Failed to encode timestamp")
	}

	createFakeLeaf(db, logID.logID, dummyHash, data, signedTimestampBytes, sequenceNumber, t)

	hashes := []trillian.Hash{dummyHash}
	leaves, err := tx.GetLeavesByHash(hashes)

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by hash: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyHash, data, t)
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByIndex")
	db := prepareTestLogDB(logID, t)
	defer db.Close()
	s := prepareTestLogStorage(logID, t)
	tx := beginLogTx(s, t)
	defer tx.Commit()

	data := []byte("some data")

	signedTimestampBytes, err := EncodeSignedTimestamp(signedTimestamp)

	if err != nil {
		t.Fatalf("Failed to encode timestamp")
	}

	createFakeLeaf(db, logID.logID, dummyHash, data, signedTimestampBytes, sequenceNumber, t)

	leaves, err := tx.GetLeavesByIndex([]int64{sequenceNumber})

	if err != nil {
		t.Fatalf("Unexpected error getting leaf by index: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], sequenceNumber, dummyHash, data, t)
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

	if len(root.LogId) != 0 || len(root.RootHash) != 0 || root.Signature != nil {
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
	root := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
		root := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}
		root2 := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 198765, TreeSize: 27, TreeRevision: 11, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
		root := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 11, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}
		root2 := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 198765, TreeSize: 16, TreeRevision: 13, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
	root := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
	root := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98765, TreeSize: 16, TreeRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedLogRoot(root); err != nil {
		t.Fatalf("Failed to store signed root: %v", err)
	}

	// TODO: Tidy up the log id as it looks silly chained 3 times like this
	root2 := trillian.SignedLogRoot{LogId: logID.logID.LogID, TimestampNanos: 98766, TreeSize: 16, TreeRevision: 6, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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

	if len(root.MapId) != 0 || len(root.RootHash) != 0 || root.Signature != nil {
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
	root := trillian.SignedMapRoot{MapId: mapID.mapID.MapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
			t.Fatalf("Root round trip failed: <%v> and: <%v>", root, root2)
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
	root := trillian.SignedMapRoot{MapId: mapID.mapID.MapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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
	root := trillian.SignedMapRoot{MapId: mapID.mapID.MapID, TimestampNanos: 98765, MapRevision: 5, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

	if err := tx.StoreSignedMapRoot(root); err != nil {
		t.Fatalf("Failed to store signed map root: %v", err)
	}

	// TODO: Tidy up the map id as it looks silly chained 3 times like this
	root2 := trillian.SignedMapRoot{MapId: mapID.mapID.MapID, TimestampNanos: 98766, MapRevision: 6, RootHash: []byte(dummyHash), Signature: &trillian.DigitallySigned{Signature: []byte("notempty")}}

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

	assert.Equal(t, 1, len(logIDs))
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

		if err := tx.QueueLeaves(leaves); err != nil {
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

	expected, got := logID.logID.TreeID, logIDs[0].TreeID
	if expected != got {
		t.Fatalf("Expected to see tree ID: %d but got: %d", expected, got)
	}
}

func ensureAllLeafHashesDistinct(leaves []trillian.LogLeaf, t *testing.T) {
	// All the hashes should be distinct. If only we had maps with slices as keys or sets
	// or pretty much any kind of usable data structures we could do this properly.
	for i, _ := range leaves {
		for j, _ := range leaves {
			if i != j && bytes.Equal(leaves[i].LeafHash, leaves[j].LeafHash) {
				t.Fatalf("Unexpectedly got a duplicate leaf hash: %v %v",
					leaves[i].LeafHash, leaves[j].LeafHash)
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
	db := prepareTestTreeDB(logID.logID.TreeID, t)

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, logID.logID.TreeID, logID.logID.LogID)

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
	db := prepareTestTreeDB(mapID.mapID.TreeID, t)

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, mapID.mapID.TreeID, mapID.mapID.MapID)

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
	leaves := make([]trillian.LogLeaf, 0)
	hasher := trillian.NewSHA256()

	for l := int64(0); l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l)
		leaf := trillian.LogLeaf{trillian.Leaf{
			hasher.Digest([]byte(lv)), []byte(lv), []byte(fmt.Sprintf("Extra %d", l))}, signedTimestamp, int64(startSeq + l)}
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

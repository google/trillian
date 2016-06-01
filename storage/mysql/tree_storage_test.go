package mysql

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/golang/protobuf/proto"
	"strings"
	"sync"
)

var allTables = []string{"Unsequenced", "TreeHead", "SequencedLeafData", "LeafData", "Node", "Trees"}

// logIDAndTest bundles up the test name and log ID to reduce cut and pasting
type logIDAndTest struct {
	logID trillian.LogID
	testName string
}

// Tests that access the db should each use a distinct log ID to prevent lock contention when
// run in parallel or race conditions / unexpected interactions.

var signedTimestamp = trillian.SignedEntryTimestamp{
	TimestampNanos: proto.Int64(1234567890), LogId: createLogID("sign").logID.LogID, Signature: &trillian.DigitallySigned{Signature: []byte("notempty")} }

// Parallel tests must get different log ids
var idMutex sync.Mutex
var testLogId int64

func createSomeNodes(testName string, treeID int64) []storage.Node {
	r := make([]storage.Node, 4)
	for i := range r {
		r[i].NodeID = storage.NewNodeIDWithPrefix(uint64(i), 8, 8, 8)
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		fmt.Printf("Node to store: %v\n", r[i].NodeID)
	}
	return r
}

func createLogID(testName string) logIDAndTest {
	idMutex.Lock()
	defer idMutex.Unlock()
	testLogId++

	return logIDAndTest{logID: trillian.LogID{LogID:  []byte(testName), TreeID: testLogId }, testName: testName}
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
	if bytes.Compare(hash, leaf.LeafHash) != 0 {
		t.Fatalf("Bad leaf hash in returned leaf: %v", leaf.LeafHash)
	}

	if leaf.SequenceNumber != int64(seq) {
		t.Fatalf("Bad sequenced number in returned leaf: %d", leaf.SequenceNumber)
	}

	if bytes.Compare(data, leaf.LeafValue) != 0 {
		t.Fatalf("Bad leaf data in returned leaf: %v", leaf.LeafValue)
	}
}

func TestNodeRoundTrip(t *testing.T) {
	logID := createLogID("TestNodeRoundTrip")
	db := prepareTestDB(logID, t)
	defer db.Close()
	s := prepareTestStorage(logID, t)

	nodesToStore := createSomeNodes("TestNodeRoundTrip", logID.logID.TreeID)

	{
		tx, err := s.Begin()
		if err != nil {
			t.Fatalf("Failed to Begin: %s", err)
		}

		if err := tx.SetMerkleNodes(nodesToStore, 100); err != nil {
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

		nodeIDs := make([]storage.NodeID, len(nodesToStore))
		for i := range nodesToStore {
			nodeIDs[i] = nodesToStore[i].NodeID
		}
		readNodes, err := tx.GetMerkleNodes(nodeIDs, 100)
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

func TestQueueDuplicateLeafFails(t *testing.T) {
	logID := createLogID("TestQueueDuplicateLeafFails")
	db := prepareTestDB(logID, t)
	defer db.Close()
	s := prepareTestStorage(logID, t)
	tx := beginTx(s, t)

	leaves := createTestLeaves(5, 10)
	err := tx.QueueLeaves(leaves)

	if err != nil {
		t.Errorf("Failed to queue leaves: %v", err)
		t.FailNow()
	}

	leaves2 := createTestLeaves(5, 12)
	err = tx.QueueLeaves(leaves2)

	if err == nil {
		t.Error("Allowed duplicate leaves to be inserted")
		t.FailNow()
	}

	if !strings.Contains(err.Error(), "Duplicate") {
		t.Errorf("Got the wrong type of error: %v", err)
	}
}

func TestQueueLeaves(t *testing.T) {
	logID := createLogID("TestQueueLeaves")
	db := prepareTestDB(logID, t)
	defer db.Close()
	s := prepareTestStorage(logID, t)
  tx := beginTx(s, t)

	leavesToInsert := 5
	leaves := createTestLeaves(leavesToInsert, 20)
	err := tx.QueueLeaves(leaves)

	if err != nil {
		t.Errorf("Failed to queue leaves: %v", err)
		t.FailNow()
	}

	commit(tx, t)

	// Should see the leaves in the database. There is no API to read from the
	// unsequenced data.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Unsequenced WHERE TreeID=?", logID.logID.TreeID).Scan(&count)

	if err != nil {
		t.Fatalf("Could not query row count")
	}

	if count != leavesToInsert {
		t.Errorf("Expected %d unsequenced rows but got: %d", leavesToInsert, count)
	}
}

func TestGetLeavesByHashNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByHashNotPresent")
	s := prepareTestStorage(logID, t)
	tx := beginTx(s, t)

	hashes := make([]trillian.Hash, 0)
	hashes = append(hashes, []byte("thisdoesntexist"))
	leaves, err := tx.GetLeavesByHash(hashes)

	if err != nil {
		t.Errorf("Error getting leaves by hash: %v", err)
	}

	if len(leaves) != 0 {
		t.Errorf("Expected no leaves returned but got %d", len(leaves))
	}
}

func TestGetLeavesByIndexNotPresent(t *testing.T) {
	logID := createLogID("TestGetLeavesByIndexNotPresent")
	s := prepareTestStorage(logID, t)
	tx := beginTx(s, t)

	_, err := tx.GetLeavesByIndex([]int64{99999})

	if err == nil {
		t.Errorf("Returned ok for leaf index when nothing inserted: %v", err)
	}
}

func TestGetLeavesByHash(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByHash")
	db := prepareTestDB(logID, t)
	defer db.Close()
	s := prepareTestStorage(logID, t)
	tx := beginTx(s, t)

	// Must be 32 bytes to match sha256 length if it was a real hash
	hash := []byte("hashxxxxhashxxxxhashxxxxhashxxxx")
	data := []byte("some data")
	sequenceNumber := 237

	signedTimestampBytes, err := EncodeSignedTimestamp(signedTimestamp)

	if err != nil {
		t.Fatalf("Failed to encode timestamp")
	}

	createFakeLeaf(db, logID.logID, hash, data, signedTimestampBytes, int64(sequenceNumber), t)

	hashes := make([]trillian.Hash, 0)
	hashes = append(hashes, hash)
	leaves, err := tx.GetLeavesByHash(hashes)

	if err != nil {
		t.Errorf("Unexpected error getting leaf by hash: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], int64(sequenceNumber), hash, data, t)
}

func TestGetLeavesByIndex(t *testing.T) {
	// Create fake leaf as if it had been sequenced
	logID := createLogID("TestGetLeavesByIndex")
	db := prepareTestDB(logID, t)
	defer db.Close()
	s := prepareTestStorage(logID, t)
	tx := beginTx(s, t)

	// Must be 32 bytes to match sha256 length if it was a real hash
	hash := []byte("hashxxxxhashxxxxhashxxxxhashxxxx")
	data := []byte("some data")
	sequenceNumber := 237

	signedTimestampBytes, err := EncodeSignedTimestamp(signedTimestamp)

	if err != nil {
		t.Fatalf("Failed to encode timestamp")
	}

	createFakeLeaf(db, logID.logID, hash, data, signedTimestampBytes, int64(sequenceNumber), t)

	leaves, err := tx.GetLeavesByIndex([]int64{int64(sequenceNumber)})

	if err != nil {
		t.Errorf("Unexpected error getting leaf by index: %v", err)
	}

	if len(leaves) != 1 {
		t.Fatalf("Got %d leaves but expected one", len(leaves))
	}

	checkLeafContents(leaves[0], int64(sequenceNumber), hash, data, t)
}

func openTestDBOrDie() *sql.DB {
	db, err := sql.Open("mysql", "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		panic(err)
	}

	return db
}

func createTestDB() {
	db := openTestDBOrDie()
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(23, "hi", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
	}
}

func prepareTestStorage(logID logIDAndTest, t *testing.T) storage.LogStorage {
	s, err := NewLogStorage(logID.logID, "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		t.Fatalf("Failed to open tree storage: %s", err)
	}

	return s
}

// This removes all database contents for the specified log id so tests run in a
// predictable environment. For obvious reasons this should only be allowed to run
// against test databases. This method panics if any of the deletions fails to make
// sure tests can't inadvertently succeed.
func prepareTestDB(logID logIDAndTest, t *testing.T) *sql.DB {
	db := openTestDBOrDie()

	// Wipe out anything that was there for this log id
	for _, table := range allTables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE TreeId=?", table), logID.logID.TreeID)

		if err != nil {
			t.Fatalf("Failed to delete rows in %s for %d", table, logID.logID.TreeID)
		}
	}

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, logID.logID.TreeID, logID.logID.LogID)

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
			panic(fmt.Errorf("Failed to delete rows in %s", table))
		}
	}
}

// Creates some test leaves with predictable data
func createTestLeaves(n, startSeq int) []trillian.LogLeaf {
	leaves := make([]trillian.LogLeaf, 0)
	hasher := trillian.NewSHA256()

	for l := 0; l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l)
		leaf := trillian.LogLeaf{trillian.Leaf{
			hasher.Digest([]byte(lv)), []byte(lv), []byte(fmt.Sprintf("Extra %d", l))}, signedTimestamp, int64(startSeq + l)}
		leaves = append(leaves, leaf)
	}

	return leaves
}

// Convenience methods to avoid copying out "if err != nil { blah }" all over the place
func commit(tx storage.LogTX, t *testing.T) {
	err := tx.Commit()

	if err != nil {
		t.Fatalf("Failed to commit tx: %v", err)
		t.FailNow()
	}
}

func beginTx(s storage.LogStorage, t *testing.T) (storage.LogTX){
	tx, err := s.Begin()

	if err != nil {
		t.Fatalf("Failed to begin tx: %v", err)
		t.FailNow()
	}

	return tx
}

func TestMain(m *testing.M) {
	flag.Parse()
	cleanTestDB()
	createTestDB()
	os.Exit(m.Run())
}

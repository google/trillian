package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian/storage"
)

// Parallel tests must get different log or map ids
var testLogID int64
var testMapID int64

type logIDAndTest struct {
	logID    int64
	testName string
}

type mapIDAndTest struct {
	mapID    int64
	testName string
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
	if expected, got := nodeID.String(), nodeID2.String(); expected != got {
		t.Errorf("Round trip of nodeID failed: %v %v", expected, got)
	}
}

func TestNodeRoundTrip(t *testing.T) {
	logID := createLogID("TestNodeRoundTrip")
	prepareTestLogDB(DB, logID, t)
	s := prepareTestLogStorage(DB, logID, t)

	const writeRevision = int64(100)

	nodesToStore := createSomeNodes("TestNodeRoundTrip", logID.logID)
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	ctx := context.Background()

	{
		tx, err := s.Begin(ctx, logID.logID)
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
		tx, err := s.Begin(ctx, logID.logID)

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

func forceWriteRevision(rev int64, tx storage.TreeTX) {
	mtx, ok := tx.(*logTX)
	if !ok {
		panic(nil)
	}
	mtx.treeTX.writeRevision = rev
}

func createLogID(testName string) logIDAndTest {
	atomic.AddInt64(&testLogID, 1)

	return logIDAndTest{
		logID:    testLogID,
		testName: testName,
	}
}

func createMapID(testName string) mapIDAndTest {
	atomic.AddInt64(&testMapID, 1)

	return mapIDAndTest{
		mapID:    testMapID,
		testName: testName,
	}
}

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

func openTestDBOrDie() *sql.DB {
	db, err := OpenDB("test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		panic(err)
	}
	return db
}

// cleanTestDB deletes all the entries in the database. Only use this with a test database
// TODO(gdbelvin): Migrate to testonly/integration
func cleanTestDB(db *sql.DB) {
	// Wipe out anything that was there for this log id
	for _, table := range allTables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))

		if err != nil {
			panic(fmt.Errorf("Failed to delete rows in %s: %s", table, err))
		}
	}
}

// TODO(codingllama): Migrate to Admin API
func createTestDB(db *sql.DB) {
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(23, "hi", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
	}
}

// prepareTestTreeDB removes all database contents for the specified log id so tests run in a predictable environment. For obvious reasons this should only be allowed to run against test databases. This method panics if any of the deletions fails to make sure tests can't inadvertently succeed.
// TODO(gdbelvin): Migrate to testonly/integration / create a new DB for freshness
func prepareTestTreeDB(db *sql.DB, treeID int64, t *testing.T) {
	// Wipe out anything that was there for this tree id
	for _, table := range allTables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE TreeId=?", table), treeID)

		if err != nil {
			t.Fatalf("Failed to delete rows in %s for %d: %s", table, treeID, err)
		}
	}
}

// prepareTestLogDB removes all database contents for the specified log id so tests run in a predictable environment. For obvious reasons this should only be allowed to run against test databases. This method panics if any of the deletions fails to make sure tests can't inadvertently succeed.
// TODO(codingllama): Migrate to Admin API
func prepareTestLogDB(db *sql.DB, logID logIDAndTest, t *testing.T) {
	prepareTestTreeDB(db, logID.logID, t)

	// Now put back the tree row for this log id
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(?, ?, "LOG", "SHA256", "SHA256")`, logID.logID, logID.logID)

	if err != nil {
		t.Fatalf("Failed to create tree entry for test: %v", err)
	}

}

func prepareTestLogStorage(db *sql.DB, logID logIDAndTest, t *testing.T) storage.LogStorage {
	if err := DeleteTree(logID.logID, db); err != nil {
		t.Fatalf("Failed to delete log storage: %s", err)
	}
	if err := CreateTree(logID.logID, db); err != nil {
		t.Fatalf("Failed to create new log storage: %s", err)
	}
	s, err := NewLogStorage(db)
	if err != nil {
		t.Fatalf("Failed to open log storage: %s", err)
	}
	return s
}

// DB is the database used for tests. It's initialized and closed by TestMain().
var DB *sql.DB

func TestMain(m *testing.M) {
	flag.Parse()
	DB = openTestDBOrDie()
	defer DB.Close()
	cleanTestDB(DB)
	createTestDB(DB)
	ec := m.Run()
	os.Exit(ec)
}

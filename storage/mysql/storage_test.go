package mysql

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian/storage"
)

// Parallel tests must get different log or map ids
var idMutex sync.Mutex
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

	t.Logf("1:\n%v (%s)\n2:\n%v (%s)", nodeID, nodeID.String(), *nodeID2, nodeID2.String())

	if expected, got := nodeID.String(), nodeID2.String(); expected != got {
		t.Errorf("Round trip of nodeID failed: %v %v", expected, got)
	}
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

func forceWriteRevision(rev int64, tx storage.TreeTX) {
	mtx, ok := tx.(*logTX)
	if !ok {
		panic(nil)
	}
	mtx.treeTX.writeRevision = rev
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

func createTestDB() {
	db := openTestDBOrDie()
	_, err := db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(23, "hi", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
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

func TestMain(m *testing.M) {
	flag.Parse()
	cleanTestDB()
	createTestDB()
	os.Exit(m.Run())
}

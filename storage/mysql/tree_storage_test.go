package mysql

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"os"
	"testing"
)

func createSomeNodes() []storage.Node {
	r := make([]storage.Node, 4)
	for i := range r {
		r[i].NodeID = storage.NewNodeIDWithPrefix(uint64(i), 8, 8, 8)
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		fmt.Printf("Node to store: %v\n", r[i].NodeID)
	}
	return r
}

func createLogID() trillian.LogID {
	return trillian.LogID{
		LogID:  []byte("hi"),
		TreeID: 23,
	}
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

// TODO: These can be cleaned up a bit when other database test PRs have been submitted
// and they can use the storage setup methods
func createLogID2() trillian.LogID {
	return trillian.LogID{
		LogID:  []byte("hi2"),
		TreeID: 24,
	}
}

func TestReadOnlyIsEnforced(t *testing.T) {
	s, err := NewLogStorage(createLogID2(), "test:zaphod@tcp(127.0.0.1:3306)/test")

	if err != nil {
		t.Fatalf("Failed to open tree storage: %s", err)
	}

	// This should fail as it's readonly
	_, err = s.Begin()
	if err != storage.ErrReadOnly {
		t.Fatalf("Did not get expected read only error: %v", err)
	}
}

func TestReadOnlyAllowsSnapshot(t *testing.T) {
	s, err := NewLogStorage(createLogID2(), "test:zaphod@tcp(127.0.0.1:3306)/test")

	if err != nil {
		t.Fatalf("Failed to open tree storage: %s", err)
	}

	// This should be ok for a readonly tree
	_, err = s.Snapshot()
	if err != nil {
		t.Fatalf("Did not allow snapshot: %v", err)
	}
}

// TODO: End of section that needs cleanup


func TestNodeRoundTrip(t *testing.T) {
	s, err := NewLogStorage(createLogID(), "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		t.Fatalf("Failed to open tree storage: %s", err)
	}

	nodesToStore := createSomeNodes()

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

func createTestDB() {
	db, err := sql.Open("mysql", "test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(23, "hi", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
	}
	// Create a second tree set up to be read only. Have to remove tree control row manually
	// to avoid a constraint issue
	_, err = db.Exec("DELETE FROM TreeControl WHERE TreeId=24")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`REPLACE INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType)
					 VALUES(24, "hi2", "LOG", "SHA256", "SHA256")`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`INSERT INTO TreeControl(TreeId, ReadOnlyRequests, SigningEnabled, SequencingEnabled, SequenceIntervalSeconds, SignIntervalSeconds)
					 VALUES(24, true, true, true, 9000, 9000)`)
	if err != nil {
		panic(err)
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	createTestDB()
	os.Exit(m.Run())
}

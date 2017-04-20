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
	"context"
	"crypto"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/testonly"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
)

func TestNodeRoundTrip(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	var writeRevision int64
	nodesToStore := createSomeNodes()
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	{
		tx := testonly.BeginLogTx(s, logID, t)
		writeRevision = tx.WriteRevision()
		defer tx.Close()

		// Need to read nodes before attempting to write
		if _, err := tx.GetMerkleNodes(ctx, writeRevision-1, nodeIDsToRead); err != nil {
			t.Fatalf("Failed to read nodes: %s", err)
		}
		if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit nodes: %s", err)
		}
	}

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()

		readNodes, err := tx.GetMerkleNodes(ctx, writeRevision, nodeIDsToRead)
		if err != nil {
			t.Fatalf("Failed to retrieve nodes: %s", err)
		}
		if err := testonly.NodesAreEqual(readNodes, nodesToStore); err != nil {
			t.Fatalf("Read back different nodes from the ones stored: %s", err)
		}
		testonly.Commit(tx, t)
	}
}

// This test ensures that node writes cross subtree boundaries so this edge case in the subtree
// cache gets exercised. Any tree size > 256 will do this.
func TestLogNodeRoundTripMultiSubtree(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(dbWrapper)
	logID := testonly.CreateLogForTest(dbWrapper)
	s := coresql.NewLogStorage(dbWrapper)

	var writeRevision int64
	nodesToStore, err := createLogNodesForTreeAtSize(871, writeRevision)
	if err != nil {
		t.Fatalf("failed to create test tree: %v", err)
	}
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	{
		tx := testonly.BeginLogTx(s, logID, t)
		writeRevision = tx.WriteRevision()
		defer tx.Close()

		// Need to read nodes before attempting to write
		if _, err := tx.GetMerkleNodes(ctx, writeRevision-1, nodeIDsToRead); err != nil {
			t.Fatalf("Failed to read nodes: %s", err)
		}
		if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit nodes: %s", err)
		}
	}

	{
		tx := testonly.BeginLogTx(s, logID, t)
		defer tx.Close()

		readNodes, err := tx.GetMerkleNodes(ctx, writeRevision, nodeIDsToRead)
		if err != nil {
			t.Fatalf("Failed to retrieve nodes: %s", err)
		}
		if err := testonly.NodesAreEqual(readNodes, nodesToStore); err != nil {
			missing, extra, err := testonly.DiffNodes(readNodes, nodesToStore)
			if err != nil {
				t.Fatalf("Failed to diff nodes: %v", err)
			}
			for _, n := range missing {
				t.Errorf("Missing: %s %s", n.NodeID.String(), n.NodeID.CoordString())
			}
			for _, n := range extra {
				t.Errorf("Extra  : %s %s", n.NodeID.String(), n.NodeID.CoordString())
			}
			t.Fatalf("Read back different nodes from the ones stored: %s", err)
		}
		testonly.Commit(tx, t)
	}
}

func createSomeNodes() []storage.Node {
	r := make([]storage.Node, 4)
	for i := range r {
		r[i].NodeID = storage.NewNodeIDWithPrefix(uint64(i), 8, 8, 8)
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		glog.Infof("Node to store: %v\n", r[i].NodeID)
	}
	return r
}

func createLogNodesForTreeAtSize(ts, rev int64) ([]storage.Node, error) {
	tree := merkle.NewCompactMerkleTree(rfc6962.TreeHasher{Hash: crypto.SHA256})
	nodeMap := make(map[string]storage.Node)
	for l := 0; l < int(ts); l++ {
		// We're only interested in the side effects of adding leaves - the node updates
		_, _, err := tree.AddLeaf([]byte(fmt.Sprintf("Leaf %d", l)), func(depth int, index int64, hash []byte) error {
			nID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, 64)

			if err != nil {
				return fmt.Errorf("failed to create a nodeID for tree - should not happen d:%d i:%d",
					depth, index)
			}

			nodeMap[nID.String()] = storage.Node{NodeID: nID, NodeRevision: rev, Hash: hash}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Unroll the map, which has deduped the updates for us and retained the latest
	nodes := make([]storage.Node, 0, len(nodeMap))
	for _, v := range nodeMap {
		nodes = append(nodes, v)
	}

	return nodes, nil
}

func openTestDBOrDie() *sql.DB {
	db, err := OpenDB("test:zaphod@tcp(127.0.0.1:3306)/test")
	if err != nil {
		panic(err)
	}
	return db
}

// cleanTestDB deletes all the entries in the database.
func cleanTestDB(wrapper wrapper.DBWrapper) {
	for _, table := range allTables {
		if _, err := wrapper.DB().ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Errorf("Failed to delete rows in %s: %s", table, err))
		}
	}
}

func OpenDB(dbURL string) (*sql.DB, error) {
	return sql.Open("mysql", dbURL)
}

// DB is the database used for tests. It's initialized and closed by TestMain().
var dbWrapper wrapper.DBWrapper

func TestMain(m *testing.M) {
	flag.Parse()
	dbWrapper = NewWrapper(openTestDBOrDie())
	defer dbWrapper.DB().Close()
	cleanTestDB(dbWrapper)
	ec := m.Run()
	os.Exit(ec)
}

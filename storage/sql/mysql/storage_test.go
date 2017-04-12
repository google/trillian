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
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/testonly"
	storageto "github.com/google/trillian/storage/testonly"
)

func TestNodeRoundTrip(t *testing.T) {
	ctx := context.Background()

	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := coresql.NewLogStorage(NewWrapper(DB))

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

	cleanTestDB(DB)
	logID := createLogForTests(DB)
	s := coresql.NewLogStorage(NewWrapper(DB))

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
			missing, extra := testonly.DiffNodes(readNodes, nodesToStore)
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
func cleanTestDB(db *sql.DB) {
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Errorf("Failed to delete rows in %s: %s", table, err))
		}
	}
}

// createMapForTests creates a map-type tree for tests. Returns the treeID of the new tree.
func createMapForTests(db *sql.DB) int64 {
	tree, err := createTree(db, storageto.MapTree)
	if err != nil {
		panic(fmt.Sprintf("Error creating map: %v", err))
	}
	return tree.TreeId
}

// createLogForTests creates a log-type tree for tests. Returns the treeID of the new tree.
func createLogForTests(db *sql.DB) int64 {
	tree, err := createTree(db, storageto.LogTree)
	if err != nil {
		panic(fmt.Sprintf("Error creating log: %v", err))
	}
	return tree.TreeId
}

// createTree creates the specified tree using AdminStorage.
func createTree(db *sql.DB, tree *trillian.Tree) (*trillian.Tree, error) {
	s := coresql.NewAdminStorage(NewWrapper(db))
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	newTree, err := tx.CreateTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newTree, nil
}

// updateTree updates the specified tree using AdminStorage.
func updateTree(db *sql.DB, treeID int64, updateFn func(*trillian.Tree)) (*trillian.Tree, error) {
	s := coresql.NewAdminStorage(NewWrapper(db))
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	tree, err := tx.UpdateTree(ctx, treeID, updateFn)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tree, nil
}

func OpenDB(dbURL string) (*sql.DB, error) {
	return sql.Open("mysql", dbURL)
}

// DB is the database used for tests. It's initialized and closed by TestMain().
var DB *sql.DB

func TestMain(m *testing.M) {
	flag.Parse()
	DB = openTestDBOrDie()
	defer DB.Close()
	cleanTestDB(DB)
	ec := m.Run()
	os.Exit(ec)
}

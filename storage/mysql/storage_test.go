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
	"crypto"
	"crypto/sha256"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
)

func TestNodeRoundTrip(t *testing.T) {
	cleanTestDB(DB)
	tree := createTreeOrPanic(DB, storageto.LogTree)
	s := NewLogStorage(DB, nil)

	const writeRevision = int64(100)
	nodesToStore := createSomeNodes()
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			forceWriteRevision(writeRevision, tx)

			// Need to read nodes before attempting to write
			if _, err := tx.GetMerkleNodes(ctx, 99, nodeIDsToRead); err != nil {
				t.Fatalf("Failed to read nodes: %s", err)
			}
			if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
				t.Fatalf("Failed to store nodes: %s", err)
			}
			return nil
		})
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			readNodes, err := tx.GetMerkleNodes(ctx, 100, nodeIDsToRead)
			if err != nil {
				t.Fatalf("Failed to retrieve nodes: %s", err)
			}
			if err := nodesAreEqual(readNodes, nodesToStore); err != nil {
				t.Fatalf("Read back different nodes from the ones stored: %s", err)
			}
			return nil
		})
	}
}

// This test ensures that node writes cross subtree boundaries so this edge case in the subtree
// cache gets exercised. Any tree size > 256 will do this.
func TestLogNodeRoundTripMultiSubtree(t *testing.T) {
	cleanTestDB(DB)
	tree := createTreeOrPanic(DB, storageto.LogTree)
	s := NewLogStorage(DB, nil)

	const writeRevision = int64(100)
	nodesToStore, err := createLogNodesForTreeAtSize(t, 871, writeRevision)
	if err != nil {
		t.Fatalf("failed to create test tree: %v", err)
	}
	nodeIDsToRead := make([]storage.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].NodeID
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			forceWriteRevision(writeRevision, tx)

			// Need to read nodes before attempting to write
			if _, err := tx.GetMerkleNodes(ctx, writeRevision-1, nodeIDsToRead); err != nil {
				t.Fatalf("Failed to read nodes: %s", err)
			}
			if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
				t.Fatalf("Failed to store nodes: %s", err)
			}
			return nil
		})
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			readNodes, err := tx.GetMerkleNodes(ctx, 100, nodeIDsToRead)
			if err != nil {
				t.Fatalf("Failed to retrieve nodes: %s", err)
			}
			if err := nodesAreEqual(readNodes, nodesToStore); err != nil {
				missing, extra := diffNodes(readNodes, nodesToStore)
				for _, n := range missing {
					t.Errorf("Missing: %s %s", n.NodeID.String(), n.NodeID.CoordString())
				}
				for _, n := range extra {
					t.Errorf("Extra  : %s %s", n.NodeID.String(), n.NodeID.CoordString())
				}
				t.Fatalf("Read back different nodes from the ones stored: %s", err)
			}
			return nil
		})
	}
}

func forceWriteRevision(rev int64, tx storage.TreeTX) {
	mtx, ok := tx.(*logTreeTX)
	if !ok {
		panic(nil)
	}
	mtx.treeTX.writeRevision = rev
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

func createLogNodesForTreeAtSize(t *testing.T, ts, rev int64) ([]storage.Node, error) {
	tree := compact.NewTree(rfc6962.New(crypto.SHA256))
	nodeMap := make(map[compact.NodeID][]byte)
	for l := 0; l < int(ts); l++ {
		// We're only interested in the side effects of adding leaves - the node updates
		if _, _, err := tree.AddLeaf([]byte(fmt.Sprintf("Leaf %d", l)), func(level uint, index uint64, hash []byte) {
			nodeMap[compact.NodeID{Level: level, Index: index}] = hash
		}); err != nil {
			return nil, err
		}
	}

	// Unroll the map, which has deduped the updates for us and retained the latest
	nodes := make([]storage.Node, 0, len(nodeMap))
	for id, hash := range nodeMap {
		nID, err := storage.NewNodeIDForTreeCoords(int64(id.Level), int64(id.Index), 64)
		if err != nil {
			t.Fatalf("failed to create NodeID for %+v: %v", id, err)
		}
		node := storage.Node{NodeID: nID, Hash: hash, NodeRevision: rev}
		nodes = append(nodes, node)
	}

	return nodes, nil
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
			return fmt.Errorf("Hashes are not the same for %s,\nlhs = %v,\nrhs = %v", lhs[i].NodeID.CoordString(), l, r)
		}
	}
	return nil
}

func diffNodes(got, want []storage.Node) ([]storage.Node, []storage.Node) {
	var missing []storage.Node
	gotMap := make(map[string]storage.Node)
	for _, n := range got {
		gotMap[n.NodeID.String()] = n
	}
	for _, n := range want {
		_, ok := gotMap[n.NodeID.String()]
		if !ok {
			missing = append(missing, n)
		}
		delete(gotMap, n.NodeID.String())
	}
	// Unpack the extra nodes to return both as slices
	extra := make([]storage.Node, 0, len(gotMap))
	for _, v := range gotMap {
		extra = append(extra, v)
	}
	return missing, extra
}

func openTestDBOrDie() *sql.DB {
	db, err := testdb.NewTrillianDB(context.TODO())
	if err != nil {
		panic(err)
	}
	return db
}

// cleanTestDB deletes all the entries in the database.
func cleanTestDB(db *sql.DB) {
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}

func createFakeSignedLogRoot(db *sql.DB, tree *trillian.Tree, treeSize uint64) {
	signer := tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notnil")), crypto.SHA256)

	ctx := context.Background()
	l := NewLogStorage(db, nil)
	err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		root, err := signer.SignLogRoot(&types.LogRootV1{TreeSize: treeSize, RootHash: []byte{0}})
		if err != nil {
			return fmt.Errorf("error creating new SignedLogRoot: %v", err)
		}
		if err := tx.StoreSignedLogRoot(ctx, *root); err != nil {
			return fmt.Errorf("error storing new SignedLogRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("ReadWriteTransaction() = %v", err))
	}
}

// createTree creates the specified tree using AdminStorage.
func createTree(db *sql.DB, tree *trillian.Tree) (*trillian.Tree, error) {
	ctx := context.Background()
	s := NewAdminStorage(db)
	tree, err := storage.CreateTree(ctx, s, tree)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func createTreeOrPanic(db *sql.DB, create *trillian.Tree) *trillian.Tree {
	tree, err := createTree(db, create)
	if err != nil {
		panic(fmt.Sprintf("Error creating tree: %v", err))
	}
	return tree
}

// updateTree updates the specified tree using AdminStorage.
func updateTree(db *sql.DB, treeID int64, updateFn func(*trillian.Tree)) (*trillian.Tree, error) {
	ctx := context.Background()
	s := NewAdminStorage(db)
	return storage.UpdateTree(ctx, s, treeID, updateFn)
}

// DB is the database used for tests. It's initialized and closed by TestMain().
var DB *sql.DB

func TestMain(m *testing.M) {
	flag.Parse()
	if !testdb.MySQLAvailable() {
		glog.Errorf("MySQL not available, skipping all MySQL storage tests")
		return
	}
	DB = openTestDBOrDie()
	defer DB.Close()
	cleanTestDB(DB)
	ec := m.Run()
	os.Exit(ec)
}

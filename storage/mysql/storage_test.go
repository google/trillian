// Copyright 2016 Google LLC. All Rights Reserved.
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
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testdb"
	storageto "github.com/google/trillian/storage/testonly"
	stree "github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
)

func TestNodeRoundTrip(t *testing.T) {
	nodes := createSomeNodes(256)
	nodeIDs := make([]compact.NodeID, len(nodes))
	for i := range nodes {
		nodeIDs[i] = nodes[i].ID
	}

	for _, tc := range []struct {
		desc    string
		store   []stree.Node
		read    []compact.NodeID
		want    []stree.Node
		wantErr bool
	}{
		{desc: "store-4-read-4", store: nodes[:4], read: nodeIDs[:4], want: nodes[:4]},
		{desc: "store-4-read-1", store: nodes[:4], read: nodeIDs[:1], want: nodes[:1]},
		{desc: "store-2-read-4", store: nodes[:2], read: nodeIDs[:4], want: nodes[:2]},
		{desc: "store-none-read-all", store: nil, read: nodeIDs, wantErr: true},
		{desc: "store-all-read-all", store: nodes, read: nodeIDs, want: nodes},
		{desc: "store-all-read-none", store: nodes, read: nil, want: nil},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			cleanTestDB(DB)
			as := NewAdminStorage(DB)
			tree := mustCreateTree(ctx, t, as, storageto.LogTree)
			s := NewLogStorage(DB, nil)

			const writeRev = int64(100)
			runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
				forceWriteRevision(writeRev, tx)
				if err := tx.SetMerkleNodes(ctx, tc.store); err != nil {
					t.Fatalf("Failed to store nodes: %s", err)
				}
				return storeLogRoot(ctx, tx, uint64(len(tc.store)), uint64(writeRev), []byte{1, 2, 3})
			})

			runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
				readNodes, err := tx.GetMerkleNodes(ctx, tc.read)
				if err != nil && !tc.wantErr {
					t.Fatalf("Failed to retrieve nodes: %s", err)
				} else if err == nil && tc.wantErr {
					t.Fatal("Retrieving nodes succeeded unexpectedly")
				}
				if err := nodesAreEqual(readNodes, tc.want); err != nil {
					t.Fatalf("Read back different nodes from the ones stored: %s", err)
				}
				return nil
			})
		})
	}
}

// This test ensures that node writes cross subtree boundaries so this edge case in the subtree
// cache gets exercised. Any tree size > 256 will do this.
func TestLogNodeRoundTripMultiSubtree(t *testing.T) {
	ctx := context.Background()
	cleanTestDB(DB)
	as := NewAdminStorage(DB)
	tree := mustCreateTree(ctx, t, as, storageto.LogTree)
	s := NewLogStorage(DB, nil)

	const writeRev = int64(100)
	const size = 871
	nodesToStore, err := createLogNodesForTreeAtSize(t, size, writeRev)
	if err != nil {
		t.Fatalf("failed to create test tree: %v", err)
	}
	nodeIDsToRead := make([]compact.NodeID, len(nodesToStore))
	for i := range nodesToStore {
		nodeIDsToRead[i] = nodesToStore[i].ID
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			forceWriteRevision(writeRev, tx)
			if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
				t.Fatalf("Failed to store nodes: %s", err)
			}
			return storeLogRoot(ctx, tx, uint64(size), uint64(writeRev), []byte{1, 2, 3})
		})
	}

	{
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			readNodes, err := tx.GetMerkleNodes(ctx, nodeIDsToRead)
			if err != nil {
				t.Fatalf("Failed to retrieve nodes: %s", err)
			}
			if err := nodesAreEqual(readNodes, nodesToStore); err != nil {
				missing, extra := diffNodes(readNodes, nodesToStore)
				for _, n := range missing {
					t.Errorf("Missing: %v", n.ID)
				}
				for _, n := range extra {
					t.Errorf("Extra  : %v", n.ID)
				}
				t.Fatalf("Read back different nodes from the ones stored: %s", err)
			}
			return nil
		})
	}
}

func forceWriteRevision(rev int64, tx storage.LogTreeTX) {
	mtx, ok := tx.(*logTreeTX)
	if !ok {
		panic(nil)
	}
	mtx.treeTX.writeRevision = rev
}

func createSomeNodes(count int) []stree.Node {
	r := make([]stree.Node, count)
	for i := range r {
		r[i].ID = compact.NewNodeID(0, uint64(i))
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		glog.V(3).Infof("Node to store: %v", r[i].ID)
	}
	return r
}

func createLogNodesForTreeAtSize(t *testing.T, ts, rev int64) ([]stree.Node, error) {
	hasher := rfc6962.New(crypto.SHA256)
	fact := compact.RangeFactory{Hash: hasher.HashChildren}
	cr := fact.NewEmptyRange(0)

	nodeMap := make(map[compact.NodeID][]byte)
	store := func(id compact.NodeID, hash []byte) { nodeMap[id] = hash }

	for l := 0; l < int(ts); l++ {
		hash := hasher.HashLeaf([]byte(fmt.Sprintf("Leaf %d", l)))
		// Store the new leaf node, and all new perfect nodes.
		// TODO(pavelkalinnikov): Visit leaf hash in cr.Append.
		store(compact.NewNodeID(0, cr.End()), hash)
		if err := cr.Append(hash, store); err != nil {
			return nil, err
		}
	}

	// Unroll the map, which has deduped the updates for us and retained the latest
	nodes := make([]stree.Node, 0, len(nodeMap))
	for id, hash := range nodeMap {
		nodes = append(nodes, stree.Node{ID: id, Hash: hash})
	}
	return nodes, nil
}

// TODO(pavelkalinnikov): Allow nodes to be out of order.
func nodesAreEqual(lhs, rhs []stree.Node) error {
	if ls, rs := len(lhs), len(rhs); ls != rs {
		return fmt.Errorf("different number of nodes, %d vs %d", ls, rs)
	}
	for i := range lhs {
		if l, r := lhs[i].ID, rhs[i].ID; l != r {
			return fmt.Errorf("NodeIDs are not the same,\nlhs = %v,\nrhs = %v", l, r)
		}
		if l, r := lhs[i].Hash, rhs[i].Hash; !bytes.Equal(l, r) {
			return fmt.Errorf("Hashes are not the same for %v,\nlhs = %v,\nrhs = %v", lhs[i].ID, l, r)
		}
	}
	return nil
}

func diffNodes(got, want []stree.Node) ([]stree.Node, []stree.Node) {
	var missing []stree.Node
	gotMap := make(map[compact.NodeID]stree.Node)
	for _, n := range got {
		gotMap[n.ID] = n
	}
	for _, n := range want {
		_, ok := gotMap[n.ID]
		if !ok {
			missing = append(missing, n)
		}
		delete(gotMap, n.ID)
	}
	// Unpack the extra nodes to return both as slices
	extra := make([]stree.Node, 0, len(gotMap))
	for _, v := range gotMap {
		extra = append(extra, v)
	}
	return missing, extra
}

func openTestDBOrDie() (*sql.DB, func(context.Context)) {
	db, done, err := testdb.NewTrillianDB(context.TODO())
	if err != nil {
		panic(err)
	}
	return db, done
}

// cleanTestDB deletes all the entries in the database.
func cleanTestDB(db *sql.DB) {
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}

func getVersion(db *sql.DB) (string, error) {
	rows, err := db.QueryContext(context.TODO(), "SELECT @@GLOBAL.version")
	if err != nil {
		return "", fmt.Errorf("getVersion: failed to perform query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return "", errors.New("getVersion: cursor has no rows")
	}
	var v string
	if err := rows.Scan(&v); err != nil {
		return "", err
	}
	if rows.Next() {
		return "", errors.New("getVersion: too many rows returned")
	}
	return v, nil
}

func mustSignAndStoreLogRoot(ctx context.Context, t *testing.T, l storage.LogStorage, tree *trillian.Tree, treeSize uint64) {
	t.Helper()
	if err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return storeLogRoot(ctx, tx, treeSize, 0, []byte{0})
	}); err != nil {
		t.Fatalf("ReadWriteTransaction: %v", err)
	}
}

func storeLogRoot(ctx context.Context, tx storage.LogTreeTX, size, rev uint64, hash []byte) error {
	logRoot, err := (&types.LogRootV1{TreeSize: size, RootHash: hash}).MarshalBinary()
	if err != nil {
		return fmt.Errorf("error marshaling new LogRoot: %v", err)
	}
	root := &trillian.SignedLogRoot{LogRoot: logRoot}
	if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
		return fmt.Errorf("error storing new SignedLogRoot: %v", err)
	}
	return nil
}

// mustCreateTree creates the specified tree using AdminStorage.
func mustCreateTree(ctx context.Context, t *testing.T, s storage.AdminStorage, tree *trillian.Tree) *trillian.Tree {
	t.Helper()
	tree, err := storage.CreateTree(ctx, s, tree)
	if err != nil {
		t.Fatalf("storage.CreateTree(): %v", err)
	}
	return tree
}

// DB is the database used for tests. It's initialized and closed by TestMain().
var DB *sql.DB

func TestMain(m *testing.M) {
	flag.Parse()
	if !testdb.MySQLAvailable() {
		glog.Errorf("MySQL not available, skipping all MySQL storage tests")
		return
	}

	var done func(context.Context)

	DB, done = openTestDBOrDie()

	if v, err := getVersion(DB); err == nil {
		glog.Infof("MySQL version '%v'", v)
	}
	status := m.Run()
	done(context.Background())
	os.Exit(status)
}

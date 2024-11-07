// Copyright 2024 Trillian Authors. All Rights Reserved.
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

package postgresql

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	testdb "github.com/google/trillian/storage/postgresql/testdbpgx"
	storageto "github.com/google/trillian/storage/testonly"
	stree "github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"k8s.io/klog/v2"
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
		testbody := func(treeDef *trillian.Tree) {
			ctx := context.Background()
			cleanTestDB(DB)
			as := NewAdminStorage(DB)
			tree := mustCreateTree(ctx, t, as, treeDef)
			s := NewLogStorage(DB, nil)

			runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
				if err := tx.SetMerkleNodes(ctx, tc.store); err != nil {
					t.Fatalf("Failed to store nodes: %s", err)
				}
				return storeLogRoot(ctx, tx, uint64(len(tc.store)), []byte{1, 2, 3})
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
		}
		t.Run(tc.desc+"-norevisions", func(t *testing.T) {
			testbody(storageto.LogTree)
		})
	}
}

// This test ensures that node writes cross subtree boundaries so this edge case in the subtree
// cache gets exercised. Any tree size > 256 will do this.
func TestLogNodeRoundTripMultiSubtree(t *testing.T) {
	testCases := []struct {
		desc string
		tree *trillian.Tree
	}{
		{
			desc: "Revisionless",
			tree: storageto.LogTree,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctx := context.Background()
			cleanTestDB(DB)
			as := NewAdminStorage(DB)
			tree := mustCreateTree(ctx, t, as, tC.tree)
			s := NewLogStorage(DB, nil)

			const size = 871
			nodesToStore, err := createLogNodesForTreeAtSize(t, size)
			if err != nil {
				t.Fatalf("failed to create test tree: %v", err)
			}
			nodeIDsToRead := make([]compact.NodeID, len(nodesToStore))
			for i := range nodesToStore {
				nodeIDsToRead[i] = nodesToStore[i].ID
			}

			{
				runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
					if err := tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
						t.Fatalf("Failed to store nodes: %s", err)
					}
					return storeLogRoot(ctx, tx, uint64(size), []byte{1, 2, 3})
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
		})
	}
}

func createSomeNodes(count int) []stree.Node {
	r := make([]stree.Node, count)
	for i := range r {
		r[i].ID = compact.NewNodeID(0, uint64(i))
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		klog.V(3).Infof("Node to store: %v", r[i].ID)
	}
	return r
}

func createLogNodesForTreeAtSize(t *testing.T, ts int64) ([]stree.Node, error) {
	t.Helper()
	hasher := rfc6962.New(crypto.SHA256)
	fact := compact.RangeFactory{Hash: hasher.HashChildren}
	cr := fact.NewEmptyRange(0)

	nodeMap := make(map[compact.NodeID][]byte)
	store := func(id compact.NodeID, hash []byte) { nodeMap[id] = hash }

	for l := 0; l < int(ts); l++ {
		hash := hasher.HashLeaf([]byte(fmt.Sprintf("Leaf %d", l)))
		// Store the new leaf node, and all new perfect nodes.
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

// TODO(robstradling): Allow nodes to be out of order.
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

func openTestDBOrDie() (*pgxpool.Pool, func(context.Context)) {
	db, done, err := testdb.NewTrillianDB(context.TODO(), testdb.DriverPostgreSQL)
	if err != nil {
		panic(err)
	}
	return db, done
}

// cleanTestDB deletes all the entries in the database.
func cleanTestDB(db *pgxpool.Pool) {
	for _, table := range allTables {
		if _, err := db.Exec(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}

func getVersion(db *pgxpool.Pool) (string, error) {
	rows, err := db.Query(context.TODO(), "SELECT version()")
	if err != nil {
		return "", fmt.Errorf("getVersion: failed to perform query: %v", err)
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()
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
	if err = rows.Err(); err != nil {
		return "", err
	}
	return v, nil
}

func mustSignAndStoreLogRoot(ctx context.Context, t *testing.T, l storage.LogStorage, tree *trillian.Tree, treeSize uint64) {
	t.Helper()
	if err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return storeLogRoot(ctx, tx, treeSize, []byte{0})
	}); err != nil {
		t.Fatalf("ReadWriteTransaction: %v", err)
	}
}

func storeLogRoot(ctx context.Context, tx storage.LogTreeTX, size uint64, hash []byte) error {
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
var DB *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	if !testdb.PostgreSQLAvailable() {
		klog.Errorf("PostgreSQL not available, skipping all PostgreSQL storage tests")
		return
	}

	var done func(context.Context)

	DB, done = openTestDBOrDie()

	if v, err := getVersion(DB); err == nil {
		klog.Infof("PostgreSQL version '%v'", v)
	}
	status := m.Run()
	done(context.Background())
	os.Exit(status)
}

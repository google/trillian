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
	"errors"
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
	stree "github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
)

func TestNodeRoundTrip(t *testing.T) {
	nodes := createSomeNodes(256)
	nodeIDs := make([]stree.NodeID, len(nodes))
	for i := range nodes {
		nodeIDs[i] = nodes[i].NodeID
	}

	for _, tc := range []struct {
		desc  string
		store []stree.Node
		read  []stree.NodeID
		want  []stree.Node
	}{
		{desc: "store-4-read-4", store: nodes[:4], read: nodeIDs[:4], want: nodes[:4]},
		{desc: "store-4-read-1", store: nodes[:4], read: nodeIDs[:1], want: nodes[:1]},
		{desc: "store-2-read-4", store: nodes[:2], read: nodeIDs[:4], want: nodes[:2]},
		{desc: "store-none-read-all", store: nil, read: nodeIDs, want: nil},
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
			preread := make([]stree.NodeID, len(tc.store))
			for i := range tc.store {
				preread[i] = tc.store[i].NodeID
			}

			runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
				forceWriteRevision(writeRev, tx)
				// Need to read nodes before attempting to write.
				if _, err := tx.GetMerkleNodes(ctx, writeRev-1, preread); err != nil {
					t.Fatalf("Failed to read nodes: %s", err)
				}
				if err := tx.SetMerkleNodes(ctx, tc.store); err != nil {
					t.Fatalf("Failed to store nodes: %s", err)
				}
				return nil
			})

			runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
				readNodes, err := tx.GetMerkleNodes(ctx, writeRev, tc.read)
				if err != nil {
					t.Fatalf("Failed to retrieve nodes: %s", err)
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

	const writeRevision = int64(100)
	nodesToStore, err := createLogNodesForTreeAtSize(t, 871, writeRevision)
	if err != nil {
		t.Fatalf("failed to create test tree: %v", err)
	}
	nodeIDsToRead := make([]stree.NodeID, len(nodesToStore))
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

func createSomeNodes(count int) []stree.Node {
	r := make([]stree.Node, count)
	for i := range r {
		r[i].NodeID = stree.NewNodeIDFromPrefix([]byte{byte(i)}, 0, 8, 8, 8)
		h := sha256.Sum256([]byte{byte(i)})
		r[i].Hash = h[:]
		glog.Infof("Node to store: %v\n", r[i].NodeID)
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
	// Store the ephemeral nodes as well.
	if _, err := cr.GetRootHash(store); err != nil {
		return nil, err
	}

	// Unroll the map, which has deduped the updates for us and retained the latest
	nodes := make([]stree.Node, 0, len(nodeMap))
	for id, hash := range nodeMap {
		nID, err := stree.NewNodeIDForTreeCoords(int64(id.Level), int64(id.Index), 64)
		if err != nil {
			t.Fatalf("failed to create NodeID for %+v: %v", id, err)
		}
		node := stree.Node{NodeID: nID, Hash: hash, NodeRevision: rev}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// TODO(pavelkalinnikov): Allow nodes to be out of order.
func nodesAreEqual(lhs []stree.Node, rhs []stree.Node) error {
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

func diffNodes(got, want []stree.Node) ([]stree.Node, []stree.Node) {
	var missing []stree.Node
	gotMap := make(map[string]stree.Node)
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
	signer := tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notnil")), crypto.SHA256)

	err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		root, err := signer.SignLogRoot(&types.LogRootV1{TreeSize: treeSize, RootHash: []byte{0}})
		if err != nil {
			return fmt.Errorf("error creating new SignedLogRoot: %v", err)
		}
		if err := tx.StoreSignedLogRoot(ctx, root); err != nil {
			return fmt.Errorf("error storing new SignedLogRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}
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

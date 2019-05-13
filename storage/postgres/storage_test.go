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

package postgres

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
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/postgres/testdb"
	storageto "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

func TestNodeRoundTrip(t *testing.T) {
	cleanTestDB(db, t)
	tree := createTreeOrPanic(db, storageto.LogTree)
	s := NewLogStorage(db, nil)

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

func openTestDBOrDie() *sql.DB {
	db, err := testdb.NewTrillianDB(context.TODO())
	if err != nil {
		panic(err)
	}
	return db
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

func TestMain(m *testing.M) {
	flag.Parse()
	if !testdb.PGAvailable() {
		glog.Errorf("PG not available, skipping all PG storage tests")
		return
	}
	db = openTestDBOrDie()
	defer db.Close()
	os.Exit(m.Run())
}

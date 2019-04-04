// Copyright 2017 Google Inc. All Rights Reserved.
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

package mysqlqm_test

import (
	"context"
	"crypto"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"
	"github.com/kylelemons/godebug/pretty"

	tcrypto "github.com/google/trillian/crypto"
	stestonly "github.com/google/trillian/storage/testonly"
)

func TestQuotaManager_GetTokens(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		t.Fatalf("GetTestDB() returned err = %v", err)
	}
	defer db.Close()

	tree, err := createTree(ctx, db)
	if err != nil {
		t.Fatalf("createTree() returned err = %v", err)
	}

	tests := []struct {
		desc                                           string
		unsequencedRows, maxUnsequencedRows, numTokens int
		specs                                          []quota.Spec
		wantErr                                        bool
	}{
		{
			desc:               "globalWriteSingleToken",
			unsequencedRows:    10,
			maxUnsequencedRows: 20,
			numTokens:          1,
			specs:              []quota.Spec{{Group: quota.Global, Kind: quota.Write}},
		},
		{
			desc:               "globalWriteMultiToken",
			unsequencedRows:    10,
			maxUnsequencedRows: 20,
			numTokens:          5,
			specs:              []quota.Spec{{Group: quota.Global, Kind: quota.Write}},
		},
		{
			desc:               "globalWriteOverQuota1",
			unsequencedRows:    20,
			maxUnsequencedRows: 20,
			numTokens:          1,
			specs:              []quota.Spec{{Group: quota.Global, Kind: quota.Write}},
			wantErr:            true,
		},
		{
			desc:               "globalWriteOverQuota2",
			unsequencedRows:    15,
			maxUnsequencedRows: 20,
			numTokens:          10,
			specs:              []quota.Spec{{Group: quota.Global, Kind: quota.Write}},
			wantErr:            true,
		},
		{
			desc:      "unlimitedQuotas",
			numTokens: 10,
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: "dylan"},
				{Group: quota.Tree, Kind: quota.Read, TreeID: tree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
				{Group: quota.User, Kind: quota.Write, User: "dylan"},
				{Group: quota.Tree, Kind: quota.Write, TreeID: tree.TreeId},
			},
		},
	}

	for _, test := range tests {
		if err := setUnsequencedRows(ctx, db, tree, test.unsequencedRows); err != nil {
			t.Errorf("setUnsequencedRows() returned err = %v", err)
			continue
		}

		// Test general cases using select count(*) to avoid flakiness / allow for more
		// precise assertions.
		// See TestQuotaManager_GetTokens_InformationSchema for information schema tests.
		qm := &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: test.maxUnsequencedRows, UseSelectCount: true}
		err := qm.GetTokens(ctx, test.numTokens, test.specs)
		if hasErr := err == mysqlqm.ErrTooManyUnsequencedRows; hasErr != test.wantErr {
			t.Errorf("%v: GetTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}
	}
}

func TestQuotaManager_GetTokens_InformationSchema(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	maxUnsequenced := 20
	globalWriteSpec := []quota.Spec{{Group: quota.Global, Kind: quota.Write}}

	// Make both variants go through the test.
	tests := []struct {
		useSelectCount bool
	}{
		{useSelectCount: true},
		{useSelectCount: false},
	}
	for _, test := range tests {
		desc := fmt.Sprintf("useSelectCount = %v", test.useSelectCount)
		t.Run(desc, func(t *testing.T) {
			db, err := testdb.NewTrillianDB(ctx)
			if err != nil {
				t.Fatalf("NewTrillianDB() returned err = %v", err)
			}
			defer db.Close()

			tree, err := createTree(ctx, db)
			if err != nil {
				t.Fatalf("createTree() returned err = %v", err)
			}

			qm := &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: maxUnsequenced, UseSelectCount: test.useSelectCount}

			// All GetTokens() calls where leaves < maxUnsequenced should succeed:
			// information_schema may be outdated, but it should refer to a valid point in the
			// past.
			for i := 0; i < maxUnsequenced-1; i++ {
				if err := queueLeaves(ctx, db, tree, i /* firstID */, 1 /* num */); err != nil {
					t.Fatalf("queueLeaves() returned err = %v", err)
				}
				if err := qm.GetTokens(ctx, 1 /* numTokens */, globalWriteSpec); err != nil {
					t.Errorf("GetTokens() returned err = %v (%v leaves)", err, i+1)
				}
			}

			// Make leaves = maxUnsequenced
			if err := queueLeaves(ctx, db, tree, maxUnsequenced-1 /* firstID */, 1 /* num */); err != nil {
				t.Fatalf("queueLeaves() returned err = %v", err)
			}

			// Allow some time for information_schema to "catch up".
			stop := false
			timeout := time.After(1 * time.Second)
			for !stop {
				select {
				case <-timeout:
					t.Errorf("timed out")
					stop = true
				default:
					// An error means that GetTokens is working correctly
					stop = qm.GetTokens(ctx, 1 /* numTokens */, globalWriteSpec) == mysqlqm.ErrTooManyUnsequencedRows
				}
			}
		})
	}
}

func TestQuotaManager_PeekTokens(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		t.Fatalf("GetTestDB() returned err = %v", err)
	}
	defer db.Close()

	tree, err := createTree(ctx, db)
	if err != nil {
		t.Fatalf("createTree() returned err = %v", err)
	}

	unsequencedRows := 10
	maxUnsequencedRows := 1000
	wantRows := maxUnsequencedRows - unsequencedRows
	if err := setUnsequencedRows(ctx, db, tree, unsequencedRows); err != nil {
		t.Fatalf("setUnsequencedRows() returned err = %v", err)
	}

	// Test using select count(*) to allow for precise assertions without flakiness.
	qm := &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: maxUnsequencedRows, UseSelectCount: true}
	specs := allSpecs(ctx, qm, tree.TreeId)
	tokens, err := qm.PeekTokens(ctx, specs)
	if err != nil {
		t.Fatalf("PeekTokens() returned err = %v", err)
	}

	// All specs but Global/Write are infinite
	wantTokens := make(map[quota.Spec]int)
	for _, spec := range specs {
		wantTokens[spec] = quota.MaxTokens
	}
	wantTokens[quota.Spec{Group: quota.Global, Kind: quota.Write}] = wantRows

	if diff := pretty.Compare(tokens, wantTokens); diff != "" {
		t.Errorf("post-PeekTokens() diff:\n%v", diff)
	}
}

func TestQuotaManager_Noops(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		t.Fatalf("GetTestDB() returned err = %v", err)
	}
	defer db.Close()

	qm := &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: 1000}
	specs := allSpecs(ctx, qm, 10 /* treeID */)

	tests := []struct {
		desc string
		fn   func() error
	}{
		{
			desc: "PutTokens",
			fn: func() error {
				return qm.PutTokens(ctx, 10 /* numTokens */, specs)
			},
		},
		{
			desc: "ResetQuota",
			fn: func() error {
				return qm.ResetQuota(ctx, specs)
			},
		},
	}
	for _, test := range tests {
		if err := test.fn(); err != nil {
			t.Errorf("%v: got err = %v", test.desc, err)
		}
	}
}

func allSpecs(_ context.Context, _ quota.Manager, treeID int64) []quota.Spec {
	return []quota.Spec{
		{Group: quota.User, Kind: quota.Read, User: "florence"},
		{Group: quota.Tree, Kind: quota.Read, TreeID: treeID},
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.User, Kind: quota.Write, User: "florence"},
		{Group: quota.Tree, Kind: quota.Write, TreeID: treeID},
		{Group: quota.Global, Kind: quota.Write},
	}
}

func countUnsequenced(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Unsequenced").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func createTree(ctx context.Context, db *sql.DB) (*trillian.Tree, error) {
	var tree *trillian.Tree

	{
		as := mysql.NewAdminStorage(db)
		err := as.ReadWriteTransaction(ctx, func(ctx context.Context, tx storage.AdminTX) error {
			var err error
			tree, err = tx.CreateTree(ctx, stestonly.LogTree)
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	{
		ls := mysql.NewLogStorage(db, nil)
		err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
			signer := tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notempty")), crypto.SHA256)
			slr, err := signer.SignLogRoot(&types.LogRootV1{RootHash: []byte{0}})
			if err != nil {
				return err
			}
			return tx.StoreSignedLogRoot(ctx, *slr)
		})
		if err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func queueLeaves(ctx context.Context, db *sql.DB, tree *trillian.Tree, firstID, num int) error {
	hasherFn, err := trees.Hash(tree)
	if err != nil {
		return err
	}
	hasher := hasherFn.New()

	leaves := []*trillian.LogLeaf{}
	for i := 0; i < num; i++ {
		value := []byte(fmt.Sprintf("leaf-%v", firstID+i))
		hasher.Reset()
		if _, err := hasher.Write(value); err != nil {
			return err
		}
		hash := hasher.Sum(nil)
		leaves = append(leaves, &trillian.LogLeaf{
			MerkleLeafHash:   hash,
			LeafValue:        value,
			ExtraData:        []byte("extra data"),
			LeafIdentityHash: hash,
		})
	}

	ls := mysql.NewLogStorage(db, nil)
	return ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		_, err := tx.QueueLeaves(ctx, leaves, time.Now())
		return err
	})
}

func setUnsequencedRows(ctx context.Context, db *sql.DB, tree *trillian.Tree, wantRows int) error {
	count, err := countUnsequenced(ctx, db)
	if err != nil {
		return err
	}
	if count == wantRows {
		return nil
	}

	// Clear the tables and re-create leaves from scratch. It's easier than having to reason
	// about duplicate entries.
	if _, err := db.ExecContext(ctx, "DELETE FROM LeafData"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM Unsequenced"); err != nil {
		return err
	}
	if err := queueLeaves(ctx, db, tree, 0 /* firstID */, wantRows); err != nil {
		return err
	}

	// Sanity check the final count
	count, err = countUnsequenced(ctx, db)
	if err != nil {
		return err
	}
	if count != wantRows {
		return fmt.Errorf("got %v unsequenced rows, want = %v", count, wantRows)
	}

	return nil
}

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

// Package mysql provides a MySQL-based storage layer implementation.
package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/sqlutil"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
)

// These statements are fixed
const (
	insertSubtreeMultiSQL = `INSERT INTO Subtree(TreeId, SubtreeId, Nodes, SubtreeRevision) ` + placeholderSQL
	insertTreeHeadSQL     = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`

	selectSubtreeSQL = `
 SELECT x.SubtreeId, x.MaxRevision, Subtree.Nodes
 FROM (
 	SELECT n.TreeId, n.SubtreeId, max(n.SubtreeRevision) AS MaxRevision
	FROM Subtree n
	WHERE n.SubtreeId IN (` + placeholderSQL + `) AND
	 n.TreeId = ? AND n.SubtreeRevision <= ?
	GROUP BY n.TreeId, n.SubtreeId
 ) AS x
 INNER JOIN Subtree 
 ON Subtree.SubtreeId = x.SubtreeId 
 AND Subtree.SubtreeRevision = x.MaxRevision 
 AND Subtree.TreeId = x.TreeId
 AND Subtree.TreeId = ?`
	placeholderSQL = sqlutil.PlaceholderSQL
)

// mySQLTreeStorage is shared between the mySQLLog- and (forthcoming) mySQLMap-
// Storage implementations, and contains functionality which is common to both,
type mySQLTreeStorage struct {
	db *sql.DB

	stmtCache *sqlutil.StmtCache
}

// OpenDB opens a database connection for all MySQL-based storage implementations.
func OpenDB(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		klog.Warningf("Could not open MySQL database, check config: %s", err)
		return nil, err
	}

	if _, err := db.ExecContext(context.TODO(), "SET sql_mode = 'STRICT_ALL_TABLES'"); err != nil {
		klog.Warningf("Failed to set strict mode on mysql db: %s", err)
		return nil, err
	}

	return db, nil
}

func newTreeStorage(db *sql.DB, mf monitoring.MetricFactory) *mySQLTreeStorage {
	return &mySQLTreeStorage{
		db:        db,
		stmtCache: sqlutil.NewStmtCache(db, mf),
	}
}

func (m *mySQLTreeStorage) getSubtreeStmt(ctx context.Context, num int) (*sqlutil.Stmt, error) {
	return m.stmtCache.GetStmt(ctx, selectSubtreeSQL, num, "?", "?")
}

func (m *mySQLTreeStorage) setSubtreeStmt(ctx context.Context, num int) (*sqlutil.Stmt, error) {
	return m.stmtCache.GetStmt(ctx, insertSubtreeMultiSQL, num, "VALUES(?, ?, ?, ?)", "(?, ?, ?, ?)")
}

func (m *mySQLTreeStorage) beginTreeTx(ctx context.Context, tree *trillian.Tree, hashSizeBytes int, subtreeCache *cache.SubtreeCache) (treeTX, error) {
	t, err := m.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		klog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	return treeTX{
		tx:            t,
		mu:            &sync.Mutex{},
		ts:            m,
		treeID:        tree.TreeId,
		treeType:      tree.TreeType,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  subtreeCache,
		writeRevision: -1,
	}, nil
}

type treeTX struct {
	// mu ensures that tx can only be used for one query/exec at a time.
	mu            *sync.Mutex
	closed        bool
	tx            *sql.Tx
	ts            *mySQLTreeStorage
	treeID        int64
	treeType      trillian.TreeType
	hashSizeBytes int
	subtreeCache  *cache.SubtreeCache
	writeRevision int64
}

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, ids [][]byte) ([]*storagepb.SubtreeProto, error) {
	klog.V(2).Infof("getSubtrees(len(ids)=%d)", len(ids))
	klog.V(4).Infof("getSubtrees(")
	if len(ids) == 0 {
		return nil, nil
	}

	tmpl, err := t.ts.getSubtreeStmt(ctx, len(ids))
	if err != nil {
		return nil, err
	}
	stx := tmpl.WithTx(ctx, t.tx)
	defer stx.Close()

	args := make([]interface{}, 0, len(ids)+3)

	// populate args with ids.
	for _, id := range ids {
		klog.V(4).Infof("  id: %x", id)
		args = append(args, id)
	}

	args = append(args, t.treeID)
	args = append(args, treeRevision)
	args = append(args, t.treeID)

	rows, err := stx.QueryContext(ctx, args...)
	if err != nil {
		klog.Warningf("Failed to get merkle subtrees: %s", err)
		return nil, err
	}
	defer rows.Close()

	if rows.Err() != nil {
		// Nothing from the DB
		klog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(ids))

	for rows.Next() {
		var subtreeIDBytes []byte
		var subtreeRev int64
		var nodesRaw []byte
		if err := rows.Scan(&subtreeIDBytes, &subtreeRev, &nodesRaw); err != nil {
			klog.Warningf("Failed to scan merkle subtree: %s", err)
			return nil, err
		}
		var subtree storagepb.SubtreeProto
		if err := proto.Unmarshal(nodesRaw, &subtree); err != nil {
			klog.Warningf("Failed to unmarshal SubtreeProto: %s", err)
			return nil, err
		}
		if subtree.Prefix == nil {
			subtree.Prefix = []byte{}
		}
		ret = append(ret, &subtree)

		if klog.V(4).Enabled() {
			klog.Infof("  subtree: NID: %x, prefix: %x, depth: %d",
				subtreeIDBytes, subtree.Prefix, subtree.Depth)
			for k, v := range subtree.Leaves {
				b, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					klog.Errorf("base64.DecodeString(%v): %v", k, err)
				}
				klog.Infof("     %x: %x", b, v)
			}
		}
	}

	// The InternalNodes cache is possibly nil here, but the SubtreeCache (which called
	// this method) will re-populate it.
	return ret, nil
}

func (t *treeTX) storeSubtrees(ctx context.Context, subtrees []*storagepb.SubtreeProto) error {
	klog.V(2).Infof("storeSubtrees(len(subtrees)=%d)", len(subtrees))
	if klog.V(4).Enabled() {
		klog.Infof("storeSubtrees(")
		for _, s := range subtrees {
			klog.Infof("  prefix: %x, depth: %d", s.Prefix, s.Depth)
			for k, v := range s.Leaves {
				b, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					klog.Errorf("base64.DecodeString(%v): %v", k, err)
				}
				klog.Infof("     %x: %x", b, v)
			}
		}
	}
	if len(subtrees) == 0 {
		return nil
	}

	// TODO(al): probably need to be able to batch this in the case where we have
	// a really large number of subtrees to store.
	args := make([]interface{}, 0, len(subtrees))

	for _, s := range subtrees {
		s := s
		if s.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", s))
		}
		subtreeBytes, err := proto.Marshal(s)
		if err != nil {
			return err
		}
		args = append(args, t.treeID)
		args = append(args, s.Prefix)
		args = append(args, subtreeBytes)
		args = append(args, t.writeRevision)
	}

	tmpl, err := t.ts.setSubtreeStmt(ctx, len(subtrees))
	if err != nil {
		return err
	}
	stx := tmpl.WithTx(ctx, t.tx)
	defer stx.Close()

	r, err := stx.ExecContext(ctx, args...)
	if err != nil {
		klog.Warningf("Failed to set merkle subtrees: %s", err)
		return err
	}
	_, _ = r.RowsAffected()
	return nil
}

func checkResultOkAndRowCountIs(res sql.Result, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return mysqlToGRPC(err)
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected, rowsError := res.RowsAffected()

	if rowsError != nil {
		return mysqlToGRPC(rowsError)
	}

	if rowsAffected != count {
		return fmt.Errorf("expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

// getSubtreesAtRev returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesAtRev(ctx context.Context, rev int64) cache.GetSubtreesFunc {
	return func(ids [][]byte) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, rev, ids)
	}
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []tree.Node) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	rev := t.writeRevision - 1
	return t.subtreeCache.SetNodes(nodes, t.getSubtreesAtRev(ctx, rev))
}

func (t *treeTX) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.writeRevision > -1 {
		tiles, err := t.subtreeCache.UpdatedTiles()
		if err != nil {
			klog.Warningf("SubtreeCache updated tiles error: %v", err)
			return err
		}
		if err := t.storeSubtrees(ctx, tiles); err != nil {
			klog.Warningf("TX commit flush error: %v", err)
			return err
		}
	}
	t.closed = true
	if err := t.tx.Commit(); err != nil {
		klog.Warningf("TX commit error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) rollbackInternal() error {
	t.closed = true
	if err := t.tx.Rollback(); err != nil {
		klog.Warningf("TX rollback error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	err := t.rollbackInternal()
	if err != nil {
		klog.Warningf("Rollback error on Close(): %v", err)
	}
	return err
}

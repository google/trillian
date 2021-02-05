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
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
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
	placeholderSQL = "<placeholder>"
)

// mySQLTreeStorage is shared between the mySQLLog- and (forthcoming) mySQLMap-
// Storage implementations, and contains functionality which is common to both,
type mySQLTreeStorage struct {
	db *sql.DB

	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '?'
	// in the query to the statement that should be used.
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
}

// OpenDB opens a database connection for all MySQL-based storage implementations.
func OpenDB(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		glog.Warningf("Could not open MySQL database, check config: %s", err)
		return nil, err
	}

	if _, err := db.ExecContext(context.TODO(), "SET sql_mode = 'STRICT_ALL_TABLES'"); err != nil {
		glog.Warningf("Failed to set strict mode on mysql db: %s", err)
		return nil, err
	}

	return db, nil
}

func newTreeStorage(db *sql.DB) *mySQLTreeStorage {
	return &mySQLTreeStorage{
		db:         db,
		statements: make(map[string]map[int]*sql.Stmt),
	}
}

// expandPlaceholderSQL expands an sql statement by adding a specified number of '?'
// placeholder slots. At most one placeholder will be expanded.
func expandPlaceholderSQL(sql string, num int, first, rest string) string {
	if num <= 0 {
		panic(fmt.Errorf("trying to expand SQL placeholder with <= 0 parameters: %s", sql))
	}

	parameters := first + strings.Repeat(","+rest, num-1)

	return strings.Replace(sql, placeholderSQL, parameters, 1)
}

// getStmt creates and caches sql.Stmt structs based on the passed in statement
// and number of bound arguments.
// TODO(al,martin): consider pulling this all out as a separate unit for reuse
// elsewhere.
func (m *mySQLTreeStorage) getStmt(ctx context.Context, statement string, num int, first, rest string) (*sql.Stmt, error) {
	m.statementMutex.Lock()
	defer m.statementMutex.Unlock()

	if m.statements[statement] != nil {
		if m.statements[statement][num] != nil {
			// TODO(al,martin): we'll possibly need to expire Stmts from the cache,
			// e.g. when DB connections break etc.
			return m.statements[statement][num], nil
		}
	} else {
		m.statements[statement] = make(map[int]*sql.Stmt)
	}

	s, err := m.db.PrepareContext(ctx, expandPlaceholderSQL(statement, num, first, rest))
	if err != nil {
		glog.Warningf("Failed to prepare statement %d: %s", num, err)
		return nil, err
	}

	m.statements[statement][num] = s

	return s, nil
}

func (m *mySQLTreeStorage) getSubtreeStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	return m.getStmt(ctx, selectSubtreeSQL, num, "?", "?")
}

func (m *mySQLTreeStorage) setSubtreeStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	return m.getStmt(ctx, insertSubtreeMultiSQL, num, "VALUES(?, ?, ?, ?)", "(?, ?, ?, ?)")
}

func (m *mySQLTreeStorage) beginTreeTx(ctx context.Context, tree *trillian.Tree, hashSizeBytes int, subtreeCache *cache.SubtreeCache) (treeTX, error) {
	t, err := m.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		glog.Warningf("Could not start tree TX: %s", err)
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
	dirty         []*storagepb.SubtreeProto
	writeRevision int64
}

func (t *treeTX) getSubtree(ctx context.Context, treeRevision int64, nodeID tree.NodeID) (*storagepb.SubtreeProto, error) {
	s, err := t.getSubtrees(ctx, treeRevision, []tree.NodeID{nodeID})
	if err != nil {
		return nil, err
	}
	switch len(s) {
	case 0:
		return nil, nil
	case 1:
		return s[0], nil
	default:
		return nil, fmt.Errorf("got %d subtrees, but expected 1", len(s))
	}
}

func (t *treeTX) getSubtreesWithLock(ctx context.Context, rev int64, ids []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.getSubtrees(ctx, rev, ids)
}

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, nodeIDs []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
	glog.V(2).Infof("getSubtrees(len(nodeIDs)=%d)", len(nodeIDs))
	glog.V(4).Infof("getSubtrees(")
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	tmpl, err := t.ts.getSubtreeStmt(ctx, len(nodeIDs))
	if err != nil {
		return nil, err
	}
	stx := t.tx.StmtContext(ctx, tmpl)
	defer stx.Close()

	args := make([]interface{}, 0, len(nodeIDs)+3)

	// populate args with nodeIDs
	for _, nodeID := range nodeIDs {
		nodeIDBytes, err := subtreeKey(nodeID)
		if err != nil {
			return nil, err
		}
		glog.V(4).Infof("  nodeID: %x", nodeIDBytes)
		args = append(args, nodeIDBytes)
	}

	args = append(args, t.treeID)
	args = append(args, treeRevision)
	args = append(args, t.treeID)

	rows, err := stx.QueryContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to get merkle subtrees: %s", err)
		return nil, err
	}
	defer rows.Close()

	if rows.Err() != nil {
		// Nothing from the DB
		glog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(nodeIDs))

	for rows.Next() {
		var subtreeIDBytes []byte
		var subtreeRev int64
		var nodesRaw []byte
		if err := rows.Scan(&subtreeIDBytes, &subtreeRev, &nodesRaw); err != nil {
			glog.Warningf("Failed to scan merkle subtree: %s", err)
			return nil, err
		}
		var subtree storagepb.SubtreeProto
		if err := proto.Unmarshal(nodesRaw, &subtree); err != nil {
			glog.Warningf("Failed to unmarshal SubtreeProto: %s", err)
			return nil, err
		}
		if subtree.Prefix == nil {
			subtree.Prefix = []byte{}
		}
		ret = append(ret, &subtree)

		if glog.V(4) {
			glog.Infof("  subtree: NID: %x, prefix: %x, depth: %d",
				subtreeIDBytes, subtree.Prefix, subtree.Depth)
			for k, v := range subtree.Leaves {
				b, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					glog.Errorf("base64.DecodeString(%v): %v", k, err)
				}
				glog.Infof("     %x: %x", b, v)
			}
		}
	}

	// The InternalNodes cache is possibly nil here, but the SubtreeCache (which called
	// this method) will re-populate it.
	return ret, nil
}

func (t *treeTX) addSubtrees(subtrees []*storagepb.SubtreeProto) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dirty = append(t.dirty, subtrees...)
}

func (t *treeTX) storeSubtrees(ctx context.Context, subtrees []*storagepb.SubtreeProto) error {
	glog.V(2).Infof("storeSubtrees(len(subtrees)=%d)", len(subtrees))
	if glog.V(4) {
		glog.Infof("storeSubtrees(")
		for _, s := range subtrees {
			glog.Infof("  prefix: %x, depth: %d", s.Prefix, s.Depth)
			for k, v := range s.Leaves {
				b, err := base64.StdEncoding.DecodeString(k)
				if err != nil {
					glog.Errorf("base64.DecodeString(%v): %v", k, err)
				}
				glog.Infof("     %x: %x", b, v)
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
	stx := t.tx.StmtContext(ctx, tmpl)
	defer stx.Close()

	r, err := stx.ExecContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to set merkle subtrees: %s", err)
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
	return func(ids []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, rev, ids)
	}
}

// GetMerkleNodes returns the requests nodes at (or below) the passed in treeRevision.
func (t *treeTX) GetMerkleNodes(ctx context.Context, treeRevision int64, nodeIDs []tree.NodeID) ([]tree.Node, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.subtreeCache.GetNodes(nodeIDs, t.getSubtreesAtRev(ctx, treeRevision))
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []tree.Node) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, n := range nodes {
		err := t.subtreeCache.SetNodeHash(n.NodeID, n.Hash,
			func(nID tree.NodeID) (*storagepb.SubtreeProto, error) {
				return t.getSubtree(ctx, t.writeRevision, nID)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *treeTX) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.writeRevision > -1 {
		if err := t.subtreeCache.Flush(ctx, func(ctx context.Context, st []*storagepb.SubtreeProto) error {
			return t.storeSubtrees(ctx, st)
		}); err != nil {
			glog.Warningf("TX commit flush error: %v", err)
			return err
		}
		if err := t.storeSubtrees(ctx, t.dirty); err != nil {
			glog.Warningf("TX commit flush error: %v", err)
			return err
		}
	}
	t.closed = true
	if err := t.tx.Commit(); err != nil {
		glog.Warningf("TX commit error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.rollbackInternal()
}

func (t *treeTX) rollbackInternal() error {
	t.closed = true
	if err := t.tx.Rollback(); err != nil {
		glog.Warningf("TX rollback error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.closed {
		err := t.rollbackInternal()
		if err != nil {
			glog.Warningf("Rollback error on Close(): %v", err)
		}
		return err
	}
	return nil
}

func (t *treeTX) IsOpen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return !t.closed
}

// subtreeKey returns a non-nil []byte suitable for use as a primary key column
// for the subtree rooted at the passed-in node ID. Returns an error if the ID
// is not aligned to bytes.
//
// TODO(pavelkalinnikov): This function is duplicated in multiple storage
// implementations. We should create a common "tree layout" type in the
// top-level storage package and reuse it for ID/strata validation.
func subtreeKey(id tree.NodeID) ([]byte, error) {
	// TODO(pavelkalinnikov): Extend this check to verify strata boundaries.
	if id.PrefixLenBits%8 != 0 {
		return nil, fmt.Errorf("invalid subtree ID - not multiple of 8: %d", id.PrefixLenBits)
	}
	// The returned slice must not be nil, as it would correspond to NULL in SQL.
	if bytes := id.Path; bytes != nil {
		return bytes[:id.PrefixLenBits/8], nil
	}
	return []byte{}, nil
}

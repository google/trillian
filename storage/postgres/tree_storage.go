// Copyright 2018 Google Inc. All Rights Reserved.
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
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
)

const (
	placeholderSQL        = "<placeholder>"
	insertSubtreeMultiSQL = `INSERT INTO subtree(tree_id, subtree_id, nodes, subtree_revision) ` + placeholderSQL
	// TODO(RJPercival): Consider using a recursive CTE in selectSubtreeSQL
	// to get the benefits of a loose index scan, which would improve
	// performance: https://wiki.postgresql.org/wiki/Loose_indexscan
	selectSubtreeSQL = `
		SELECT x.subtree_id, x.max_revision, subtree.nodes
		FROM (
			SELECT n.subtree_id, max(n.subtree_revision) AS max_revision
			FROM subtree n
			WHERE n.subtree_id IN (` + placeholderSQL + `) AND
			n.tree_id = <param> AND n.subtree_revision <= <param>
			GROUP BY n.subtree_id
		) AS x
		INNER JOIN subtree
		ON subtree.subtree_id = x.subtree_id
		AND subtree.subtree_revision = x.max_revision
		AND subtree.tree_id = <param>`
	insertTreeHeadSQL = `INSERT INTO tree_head(tree_id,tree_head_timestamp,tree_size,root_hash,tree_revision,root_signature)
                 VALUES($1,$2,$3,$4,$5,$6)`
)

// pgTreeStorage contains the pgLogStorage implementation.
type pgTreeStorage struct {
	db *sql.DB

	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '$#'
	// in the query to the statement that should be used.
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
}

// OpenDB opens a database connection for all PG-based storage implementations.
func OpenDB(connStr string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		// Don't log conn str as it could contain credentials.
		glog.Warningf("Could not open Postgres database, check config: %s", err)
		return nil, err
	}

	return db, nil
}

func newTreeStorage(db *sql.DB) *pgTreeStorage {
	return &pgTreeStorage{
		db:         db,
		statements: make(map[string]map[int]*sql.Stmt),
	}
}

// statementSkeleton contains the structure of a query to create.
type statementSkeleton struct {
	// sql is the main query with an embedded placeholder.
	sql string
	// firstInsertion is the first sql query that should be inserted
	// in place of the placeholder.
	firstInsertion string
	// firstPlaceholders is the number of variables in the firstInsertion.
	// Used for string interpolation.
	firstPlaceholders int
	// restInsertion is the remaining sql query that should be repeated following
	// the first insertion.
	restInsertion string
	// restPlaceholders is the number of variables in a single restInsertion.
	// Used for string interpolation.
	restPlaceholders int
	// num is the total repetitions (firstInsertion + restInsertion * num - 1) that
	// should be inserted.
	num int
}

// expandPlaceholderSQL expands an sql statement by adding a specified number of '%s'
// placeholder slots. At most one placeholder will be expanded.
func expandPlaceholderSQL(skeleton *statementSkeleton) (string, error) {
	if skeleton.num <= 0 {
		return "", fmt.Errorf("trying to expand SQL placeholder with <= 0 parameters: %s", skeleton.sql)
	}

	restCount := skeleton.num - 1

	totalArray := make([]interface{}, skeleton.firstPlaceholders+skeleton.restPlaceholders*(restCount))
	for i := range totalArray {
		totalArray[i] = fmt.Sprintf("$%d", i+1)
	}

	toInsertBuilder := strings.Builder{}
	toInsertBuilder.WriteString(fmt.Sprintf(skeleton.firstInsertion, totalArray[:skeleton.firstPlaceholders]...))
	remainingInsertion := strings.Repeat(","+skeleton.restInsertion, restCount)
	toInsertBuilder.WriteString(fmt.Sprintf(remainingInsertion, totalArray[skeleton.firstPlaceholders:]...))

	return strings.Replace(skeleton.sql, placeholderSQL, toInsertBuilder.String(), 1), nil
}

// getStmt creates and caches sql.Stmt structs based on the passed in statement
// and number of bound arguments.
func (p *pgTreeStorage) getStmt(ctx context.Context, skeleton *statementSkeleton) (*sql.Stmt, error) {
	p.statementMutex.Lock()
	defer p.statementMutex.Unlock()

	if p.statements[skeleton.sql] != nil {
		if p.statements[skeleton.sql][skeleton.num] != nil {
			return p.statements[skeleton.sql][skeleton.num], nil
		}
	} else {
		p.statements[skeleton.sql] = make(map[int]*sql.Stmt)
	}

	statement, err := expandPlaceholderSQL(skeleton)

	counter := skeleton.restPlaceholders*skeleton.num + 1
	for strings.Contains(statement, "<param>") {
		statement = strings.Replace(statement, "<param>", "$"+strconv.Itoa(counter), 1)
		counter++
	}

	if err != nil {
		glog.Warningf("Failed to expand placeholder sql: %v", skeleton)
		return nil, err
	}
	s, err := p.db.PrepareContext(ctx, statement)

	if err != nil {
		glog.Warningf("Failed to prepare statement %d: %s", skeleton.num, err)
		return nil, err
	}

	p.statements[skeleton.sql][skeleton.num] = s

	return s, nil
}

func (p *pgTreeStorage) getSubtreeStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	skeleton := &statementSkeleton{
		sql:               selectSubtreeSQL,
		firstInsertion:    "%s",
		firstPlaceholders: 1,
		restInsertion:     "%s",
		restPlaceholders:  1,
		num:               num,
	}
	return p.getStmt(ctx, skeleton)
}

func (p *pgTreeStorage) setSubtreeStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	skeleton := &statementSkeleton{
		sql:               insertSubtreeMultiSQL,
		firstInsertion:    "VALUES(%s, %s, %s, %s)",
		firstPlaceholders: 4,
		restInsertion:     "(%s, %s, %s, %s)",
		restPlaceholders:  4,
		num:               num,
	}
	return p.getStmt(ctx, skeleton)
}

func (p *pgTreeStorage) beginTreeTx(ctx context.Context, tree *trillian.Tree, hashSizeBytes int, subtreeCache *cache.SubtreeCache) (treeTX, error) {
	t, err := p.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		glog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	return treeTX{
		tx:            t,
		ts:            p,
		treeID:        tree.TreeId,
		treeType:      tree.TreeType,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  subtreeCache,
		writeRevision: -1,
	}, nil
}

type treeTX struct {
	closed        bool
	tx            *sql.Tx
	ts            *pgTreeStorage
	treeID        int64
	treeType      trillian.TreeType
	hashSizeBytes int
	subtreeCache  *cache.SubtreeCache
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

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, nodeIDs []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
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

	// Populate args with node IDs.
	for _, nodeID := range nodeIDs {
		nodeIDBytes, err := subtreeKey(nodeID)
		if err != nil {
			return nil, err
		}
		glog.V(4).Infof("  nodeID: %x", nodeIDBytes)
		args = append(args, nodeIDBytes)
	}

	args = append(args, interface{}(t.treeID))
	args = append(args, interface{}(treeRevision))
	args = append(args, interface{}(t.treeID))
	rows, err := stx.QueryContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to get merkle subtrees: QueryContext(%v) = (_, %q)", args, err)
		return nil, err
	}
	defer rows.Close()

	if rows.Err() != nil {
		// Nothing from the DB.
		glog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(nodeIDs))

	for rows.Next() {
		var subtreeIDBytes []byte
		var subtreeRev int64
		var nodesRaw []byte
		var subtree storagepb.SubtreeProto
		if err := rows.Scan(&subtreeIDBytes, &subtreeRev, &nodesRaw); err != nil {
			glog.Warningf("Failed to scan merkle subtree: %s", err)
			return nil, err
		}
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

func (t *treeTX) storeSubtrees(ctx context.Context, subtrees []*storagepb.SubtreeProto) error {
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
		glog.Warning("attempted to store 0 subtrees...")
		return nil
	}

	args := make([]interface{}, 0, len(subtrees))

	for _, s := range subtrees {
		st := s
		if st.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", st))
		}
		subtreeBytes, err := proto.Marshal(st)
		if err != nil {
			return err
		}
		args = append(args, t.treeID)
		args = append(args, st.Prefix)
		args = append(args, subtreeBytes)
		args = append(args, t.writeRevision)
	}

	tmpl, err := t.ts.setSubtreeStmt(ctx, len(subtrees))
	if err != nil {
		return err
	}
	stx := t.tx.StmtContext(ctx, tmpl)
	defer stx.Close()

	_, err = stx.ExecContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to set merkle subtrees: %s", err)
		return err
	}
	return nil
}

func (t *treeTX) Commit(ctx context.Context) error {
	if t.writeRevision > -1 {
		if err := t.subtreeCache.Flush(ctx, func(ctx context.Context, st []*storagepb.SubtreeProto) error {
			return t.storeSubtrees(ctx, st)
		}); err != nil {
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
	t.closed = true
	if err := t.tx.Rollback(); err != nil {
		glog.Warningf("TX rollback error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) Close() error {
	if !t.closed {
		err := t.Rollback()
		if err != nil {
			glog.Warningf("Rollback error on Close(): %v", err)
		}
		return err
	}
	return nil
}

func (t *treeTX) GetMerkleNodes(ctx context.Context, treeRevision int64, nodeIDs []tree.NodeID) ([]tree.Node, error) {
	return t.subtreeCache.GetNodes(nodeIDs, t.getSubtreesAtRev(ctx, treeRevision))
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []tree.Node) error {
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

func (t *treeTX) IsOpen() bool {
	return !t.closed
}

// getSubtreesAtRev returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesAtRev(ctx context.Context, rev int64) cache.GetSubtreesFunc {
	return func(ids []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, rev, ids)
	}
}

func checkResultOkAndRowCountIs(res sql.Result, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return err
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected, rowsError := res.RowsAffected()

	if rowsError != nil {
		return rowsError
	}

	if rowsAffected != count {
		return fmt.Errorf("expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

// subtreeKey returns a non-nil []byte suitable for use as a primary key column
// for the subtree rooted at the passed-in node ID. Returns an error if the ID
// is not aligned to bytes.
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

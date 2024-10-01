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

// Package postgresql provides a PostgreSQL-based storage layer implementation.
package postgresql

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/postgresql/postgresqlpb"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/klog/v2"
)

// These statements are fixed
const (
	createTempSubtreeTable = `CREATE TEMP TABLE TempSubtree (
	TreeId BIGINT,
	SubtreeId BYTEA,
	Nodes BYTEA,
	SubtreeRevision INTEGER,
	CONSTRAINT TempSubtree_pk PRIMARY KEY (TreeId,SubtreeId,SubtreeRevision)
) ON COMMIT DROP`
	insertSubtreeMultiSQL = `INSERT INTO Subtree(TreeId, SubtreeId, Nodes, SubtreeRevision) SELECT TreeId, SubtreeId, Nodes, SubtreeRevision FROM TempSubtree ON CONFLICT ON CONSTRAINT TempSubtree_pk DO UPDATE Nodes=EXCLUDED.Nodes`
	insertTreeHeadSQL     = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`

	selectSubtreeSQL = `
 SELECT x.SubtreeId, Subtree.Nodes
 FROM (
 	SELECT n.TreeId, n.SubtreeId, max(n.SubtreeRevision) AS MaxRevision
	FROM Subtree n
	WHERE n.SubtreeId = ANY(?) AND
	 n.TreeId = ? AND n.SubtreeRevision <= ?
	GROUP BY n.TreeId, n.SubtreeId
 ) AS x
 INNER JOIN Subtree 
 ON Subtree.SubtreeId = x.SubtreeId 
 AND Subtree.SubtreeRevision = x.MaxRevision 
 AND Subtree.TreeId = x.TreeId
 AND Subtree.TreeId = ?`

	selectSubtreeSQLNoRev = `
 SELECT SubtreeId, Subtree.Nodes
 FROM Subtree
 WHERE Subtree.TreeId = ?
   AND SubtreeId = ANY(?)`
	placeholderSQL = "<placeholder>"
)

// postgreSQLTreeStorage is shared between the postgreSQLLog- and (forthcoming) postgreSQLMap-
// Storage implementations, and contains functionality which is common to both,
type postgreSQLTreeStorage struct {
	db *pgxpool.Pool

	// pgx automatically prepares and caches statements, so there is no need for
	// a statement map in this struct.
	// (See https://github.com/jackc/pgx/wiki/Automatic-Prepared-Statement-Caching)
}

// OpenDB opens a database connection for all PostgreSQL-based storage implementations.
func OpenDB(dbURL string) (*pgxpool.Pool, error) {
	db, err := pgxpool.New("postgresql", dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		klog.Warningf("Could not open PostgreSQL database, check config: %s", err)
		return nil, err
	}

	return db, nil
}

func newTreeStorage(db *pgxpool.Pool) *postgreSQLTreeStorage {
	return &postgreSQLTreeStorage{
		db: db,
	}
}

func (m *postgreSQLTreeStorage) beginTreeTx(ctx context.Context, tree *trillian.Tree, hashSizeBytes int, subtreeCache *cache.SubtreeCache) (treeTX, error) {
	t, err := m.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		klog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	var subtreeRevisions bool
	o := &postgresqlpb.StorageOptions{}
	if err := anypb.UnmarshalTo(tree.StorageSettings, o, proto.UnmarshalOptions{}); err != nil {
		return treeTX{}, fmt.Errorf("failed to unmarshal StorageSettings: %v", err)
	}
	subtreeRevisions = o.SubtreeRevisions
	return treeTX{
		tx:            t,
		mu:            &sync.Mutex{},
		ts:            m,
		treeID:        tree.TreeId,
		treeType:      tree.TreeType,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  subtreeCache,
		writeRevision: -1,
		subtreeRevs:   subtreeRevisions,
	}, nil
}

type treeTX struct {
	// mu ensures that tx can only be used for one query/exec at a time.
	mu            *sync.Mutex
	closed        bool
	tx            pgx.Tx
	ts            *postgreSQLTreeStorage
	treeID        int64
	treeType      trillian.TreeType
	hashSizeBytes int
	subtreeCache  *cache.SubtreeCache
	writeRevision int64
	subtreeRevs   bool
}

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, ids [][]byte) ([]*storagepb.SubtreeProto, error) {
	klog.V(2).Infof("getSubtrees(len(ids)=%d)", len(ids))
	klog.V(4).Infof("getSubtrees(")
	if len(ids) == 0 {
		return nil, nil
	}

	var rows pgx.Rows
	var err error
	if t.subtreeRevs {
		rows, err = t.tx.Query(ctx, selectSubtreeSQL, ids, t.treeID, treeRevision, t.treeID)
	} else {
		rows, err = t.tx.Query(ctx, selectSubtreeSQLNoRev, t.treeID, ids)
	}
	if err != nil {
		klog.Warningf("Failed to get merkle subtrees: %s", err)
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			klog.Errorf("rows.Close(): %v", err)
		}
	}()

	if rows.Err() != nil {
		// Nothing from the DB
		klog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(ids))

	for rows.Next() {
		var subtreeIDBytes []byte
		var nodesRaw []byte
		if err := rows.Scan(&subtreeIDBytes, &nodesRaw); err != nil {
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

	if err := rows.Err(); err != nil {
		return nil, err
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
	rows := make([][]interface{}, 0, len(subtrees))

	// If not using subtree revisions then default value of 0 is fine. There is no
	// significance to this value, other than it cannot be NULL in the DB.
	var subtreeRev int64
	if t.subtreeRevs {
		// We're using subtree revisions, so ensure we write at the correct revision
		subtreeRev = t.writeRevision
	}
	for _, s := range subtrees {
		s := s
		if s.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", s))
		}
		subtreeBytes, err := proto.Marshal(s)
		if err != nil {
			return err
		}
		rows = append(rows, []interface{}{t.treeID, s.Prefix, subtreeBytes, subtreeRev})
	}

	// Create temporary subtree table.
	_, err := t.tx.Exec(ctx, createTempSubtreeTable)
	if err != nil {
		klog.Warningf("Failed to create temporary subtree table: %s", err)
		return err
	}

	// Copy subtrees to temporary table.
	_, err = t.tx.CopyFrom(
		ctx,
		pgx.Identifier{"TempSubtree"},
		[]string{"TreeId", "SubtreeId", "Nodes", "SubtreeRevision"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		klog.Warningf("Failed to copy merkle subtrees: %s", err)
		return err
	}

	// Upsert the subtrees.
	_, err = t.tx.Exec(ctx, insertSubtreeMultiSQL)
	if err != nil {
		klog.Warningf("Failed to set merkle subtrees: %s", err)
		return err
	}
	_, _ = r.RowsAffected()
	return nil
}

func checkResultOkAndRowCountIs(res pgconn.CommandTag, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return postgresqlToGRPC(err)
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected, rowsError := res.RowsAffected()

	if rowsError != nil {
		return postgresqlToGRPC(rowsError)
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
	if err := t.tx.Commit(ctx); err != nil {
		klog.Warningf("TX commit error: %s, stack:\n%s", err, string(debug.Stack()))
		return err
	}
	return nil
}

func (t *treeTX) rollbackInternal() error {
	t.closed = true
	if err := t.tx.Rollback(context.TODO()); err != nil {
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

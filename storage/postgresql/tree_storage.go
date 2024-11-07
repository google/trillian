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
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
)

// These statements are fixed
const (
	createTempSubtreeTable = "CREATE TEMP TABLE TempSubtree (" +
		" TreeId BIGINT," +
		" SubtreeId BYTEA," +
		" Nodes BYTEA," +
		" CONSTRAINT TempSubtree_pk PRIMARY KEY (TreeId,SubtreeId)" +
		") ON COMMIT DROP"
	insertSubtreeMultiSQL = "INSERT INTO Subtree(TreeId,SubtreeId,Nodes) " +
		"SELECT TreeId,SubtreeId,Nodes " +
		"FROM TempSubtree " +
		"ON CONFLICT ON CONSTRAINT Subtree_pk DO UPDATE SET Nodes=EXCLUDED.Nodes"
	insertTreeHeadSQL = "INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,RootSignature) " +
		"VALUES($1,$2,$3,$4,$5) " +
		"ON CONFLICT DO NOTHING"

	selectSubtreeSQL = "SELECT SubtreeId,Nodes " +
		"FROM Subtree " +
		"WHERE TreeId=$1" +
		" AND SubtreeId=ANY($2)"
)

// postgreSQLTreeStorage is shared between the postgreSQLLog- and (forthcoming) postgreSQLMap-
// Storage implementations, and contains functionality which is common to both,
type postgreSQLTreeStorage struct {
	db *pgxpool.Pool

	// pgx automatically prepares and caches statements, so there is no need for
	// a statement map in this struct.
	// (See https://github.com/jackc/pgx/wiki/Automatic-Prepared-Statement-Caching)
}

// OpenDB opens a database connection pool for all PostgreSQL-based storage implementations.
func OpenDB(dbURL string) (*pgxpool.Pool, error) {
	pgxConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		klog.Warningf("Could not parse PostgreSQL connection URI, check config: %s", err)
		return nil, err
	}

	db, err := pgxpool.NewWithConfig(context.TODO(), pgxConfig)
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

	return treeTX{
		tx:            t,
		mu:            &sync.Mutex{},
		ts:            m,
		treeID:        tree.TreeId,
		treeType:      tree.TreeType,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  subtreeCache,
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
}

func (t *treeTX) getSubtrees(ctx context.Context, ids [][]byte) ([]*storagepb.SubtreeProto, error) {
	klog.V(2).Infof("getSubtrees(len(ids)=%d)", len(ids))
	klog.V(4).Infof("getSubtrees(")
	if len(ids) == 0 {
		return nil, nil
	}

	var rows pgx.Rows
	var err error
	rows, err = t.tx.Query(ctx, selectSubtreeSQL, t.treeID, ids)
	if err != nil {
		klog.Warningf("Failed to get merkle subtrees: %s", err)
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()

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

	// TODO(robstradling): probably need to be able to batch this in the case where we have
	// a really large number of subtrees to store.
	rows := make([][]interface{}, 0, len(subtrees))

	for _, s := range subtrees {
		s := s
		if s.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", s))
		}
		subtreeBytes, err := proto.Marshal(s)
		if err != nil {
			return err
		}
		rows = append(rows, []interface{}{t.treeID, s.Prefix, subtreeBytes})
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
		pgx.Identifier{"tempsubtree"},
		[]string{"treeid", "subtreeid", "nodes"},
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
	return nil
}

func checkResultOkAndRowCountIs(res pgconn.CommandTag, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return postgresqlToGRPC(err)
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected := res.RowsAffected()
	if rowsAffected != count {
		return fmt.Errorf("expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

func checkResultOkAndCopyCountIs(rowsAffected int64, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return postgresqlToGRPC(err)
	}

	// Otherwise we have to look at the result of the operation
	if rowsAffected != count {
		return fmt.Errorf("expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

// getSubtreesFunc returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesFunc(ctx context.Context) cache.GetSubtreesFunc {
	return func(ids [][]byte) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, ids)
	}
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []tree.Node) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.subtreeCache.SetNodes(nodes, t.getSubtreesFunc(ctx))
}

func (t *treeTX) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tiles, err := t.subtreeCache.UpdatedTiles()
	if err != nil {
		klog.Warningf("SubtreeCache updated tiles error: %v", err)
		return err
	}
	if err := t.storeSubtrees(ctx, tiles); err != nil {
		klog.Warningf("TX commit flush error: %v", err)
		return err
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

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

package coresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
	"github.com/google/trillian/storage/storagepb"
)

// sqlTreeStorage is shared between the log and map storage implementations, and contains
// functionality which is common to both,
type sqlTreeStorage struct {
	wrap wrapper.DBWrapper
}

func newTreeStorage(wrapper wrapper.DBWrapper) *sqlTreeStorage {
	return &sqlTreeStorage{
		wrap: wrapper,
	}
}

func (m *sqlTreeStorage) beginTreeTx(ctx context.Context, treeID int64, hashSizeBytes int, strataDepths []int, populate storage.PopulateSubtreeFunc, prepare storage.PrepareSubtreeWriteFunc) (treeTX, error) {
	t, err := m.wrap.DB().Begin()
	if err != nil {
		glog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	return treeTX{
		tx:            t,
		ts:            m,
		treeID:        treeID,
		hashSizeBytes: hashSizeBytes,
		subtreeCache:  cache.NewSubtreeCache(strataDepths, populate, prepare),
		writeRevision: -1,
	}, nil
}

type treeTX struct {
	closed        bool
	tx            *sql.Tx
	ts            *sqlTreeStorage
	treeID        int64
	hashSizeBytes int
	subtreeCache  cache.SubtreeCache
	writeRevision int64
}

func (t *treeTX) getSubtree(ctx context.Context, treeRevision int64, nodeID storage.NodeID) (*storagepb.SubtreeProto, error) {
	s, err := t.getSubtrees(ctx, treeRevision, []storage.NodeID{nodeID})
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

func subtreeScanFn(rows *sql.Rows, num int) ([]*storagepb.SubtreeProto, error) {
	if rows.Err() != nil {
		// Nothing from the DB
		glog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, num)
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
	}

	return ret, nil
}

func (t *treeTX) getSubtrees(ctx context.Context, treeRevision int64, nodeIDs []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	subtrees, err := t.ts.wrap.GetSubtrees(t.tx, t.treeID, treeRevision, nodeIDs)
	if err != nil {
		glog.Warningf("Failed to get merkle subtrees: %v", err)
		return nil, err
	}
	// The InternalNodes cache is possibly nil here, but the SubtreeCache (which called
	// this method) will re-populate it.
	return subtrees, nil
}

func (t *treeTX) storeSubtrees(ctx context.Context, subtrees []*storagepb.SubtreeProto) error {
	if len(subtrees) == 0 {
		glog.Warning("attempted to store 0 subtrees...")
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

	stmt, err := t.ts.wrap.SetSubtreeStmt(t.tx, len(subtrees))
	if err != nil {
		return err
	}
	defer stmt.Close()

	r, err := stmt.ExecContext(ctx, args...)
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
		return err
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected, rowsError := res.RowsAffected()

	if rowsError != nil {
		return rowsError
	}

	if rowsAffected != count {
		return fmt.Errorf("Expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

// GetTreeRevisionAtSize returns the max node version for a tree at a particular size.
// It is an error to request tree sizes larger than the currently published tree size.
// For an inexact tree size this implementation always returns the next largest revision if an
// exact one does not exist but it isn't required to do so.
func (t *treeTX) GetTreeRevisionIncludingSize(ctx context.Context, treeSize int64) (int64, int64, error) {
	// Negative size is not sensible and a zero sized tree has no nodes so no revisions
	if treeSize <= 0 {
		return 0, 0, fmt.Errorf("invalid tree size: %d", treeSize)
	}

	var treeRevision, actualTreeSize int64
	stmt, err := t.ts.wrap.GetTreeRevisionIncludingSizeStmt(t.tx)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	err = stmt.QueryRowContext(ctx, t.treeID, treeSize).Scan(&treeRevision, &actualTreeSize)
	return treeRevision, actualTreeSize, err
}

// getSubtreesAtRev returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesAtRev(ctx context.Context, rev int64) cache.GetSubtreesFunc {
	return func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(ctx, rev, ids)
	}
}

// GetMerkleNodes returns the requests nodes at (or below) the passed in treeRevision.
func (t *treeTX) GetMerkleNodes(ctx context.Context, treeRevision int64, nodeIDs []storage.NodeID) ([]storage.Node, error) {
	return t.subtreeCache.GetNodes(nodeIDs, t.getSubtreesAtRev(ctx, treeRevision))
}

func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []storage.Node) error {
	for _, n := range nodes {
		err := t.subtreeCache.SetNodeHash(n.NodeID, n.Hash,
			func(nID storage.NodeID) (*storagepb.SubtreeProto, error) {
				return t.getSubtree(ctx, t.writeRevision, nID)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *treeTX) Commit() error {
	if t.writeRevision > -1 {
		if err := t.subtreeCache.Flush(func(st []*storagepb.SubtreeProto) error {
			return t.storeSubtrees(context.TODO(), st)
		}); err != nil {
			glog.Warningf("TX commit flush error: %v", err)
			return err
		}
	}
	t.closed = true
	if err := t.tx.Commit(); err != nil {
		glog.Warningf("TX commit error: %s", err)
		return err
	}
	return nil
}

func (t *treeTX) Rollback() error {
	t.closed = true
	if err := t.tx.Rollback(); err != nil {
		glog.Warningf("TX rollback error: %s", err)
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

func (t *treeTX) IsOpen() bool {
	return !t.closed
}

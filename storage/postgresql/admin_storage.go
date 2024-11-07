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
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

const (
	defaultSequenceIntervalSeconds = 60

	selectTrees = "SELECT TreeId,TreeState,TreeType,DisplayName,Description,CreateTimeMillis,UpdateTimeMillis,MaxRootDurationMillis,Deleted,DeleteTimeMillis " +
		"FROM Trees"
	selectNonDeletedTrees = selectTrees + " WHERE (Deleted IS NULL OR Deleted='false')"
	selectTreeByID        = selectTrees + " WHERE TreeId=$1"

	updateTreeSQL = "UPDATE Trees " +
		"SET TreeState=$1,TreeType=$2,DisplayName=$3,Description=$4,UpdateTimeMillis=$5,MaxRootDurationMillis=$6 " +
		"WHERE TreeId=$7"
)

// NewAdminStorage returns a PostgreSQL storage.AdminStorage implementation backed by DB.
func NewAdminStorage(db *pgxpool.Pool) *postgresqlAdminStorage {
	return &postgresqlAdminStorage{db}
}

// postgresqlAdminStorage implements storage.AdminStorage
type postgresqlAdminStorage struct {
	db *pgxpool.Pool
}

func (s *postgresqlAdminStorage) Snapshot(ctx context.Context) (storage.ReadOnlyAdminTX, error) {
	return s.beginInternal(ctx)
}

func (s *postgresqlAdminStorage) beginInternal(ctx context.Context) (storage.AdminTX, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	return &adminTX{tx: tx}, nil
}

func (s *postgresqlAdminStorage) ReadWriteTransaction(ctx context.Context, f storage.AdminTXFunc) error {
	tx, err := s.beginInternal(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Close(); err != nil {
			klog.Errorf("tx.Close(): %v", err)
		}
	}()
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresqlAdminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return s.db.Ping(ctx)
}

type adminTX struct {
	tx pgx.Tx

	// mu guards reads/writes on closed, which happen on Commit/Close methods.
	//
	// We don't check closed on methods apart from the ones above, as we trust tx
	// to keep tabs on its state, and hence fail to do queries after closed.
	mu     sync.RWMutex
	closed bool
}

func (t *adminTX) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.tx.Commit(context.TODO())
}

func (t *adminTX) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.tx.Rollback(context.TODO())
}

func (t *adminTX) GetTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	// GetTree is an entry point for most RPCs, let's provide somewhat nicer error messages.
	tree, err := readTree(t.tx.QueryRow(ctx, selectTreeByID, treeID))
	switch {
	case err == pgx.ErrNoRows:
		// ErrNoRows doesn't provide useful information, so we don't forward it.
		return nil, status.Errorf(codes.NotFound, "tree %v not found", treeID)
	case err != nil:
		return nil, fmt.Errorf("error reading tree %v: %v", treeID, err)
	}
	return tree, nil
}

func (t *adminTX) ListTrees(ctx context.Context, includeDeleted bool) ([]*trillian.Tree, error) {
	var query string
	if includeDeleted {
		query = selectTrees
	} else {
		query = selectNonDeletedTrees
	}

	rows, err := t.tx.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()
	trees := []*trillian.Tree{}
	for rows.Next() {
		tree, err := readTree(rows)
		if err != nil {
			return nil, err
		}
		trees = append(trees, tree)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return trees, nil
}

func (t *adminTX) CreateTree(ctx context.Context, tree *trillian.Tree) (*trillian.Tree, error) {
	if err := storage.ValidateTreeForCreation(ctx, tree); err != nil {
		return nil, err
	}

	id, err := storage.NewTreeID()
	if err != nil {
		return nil, err
	}

	// Use the time truncated-to-millis throughout, as that's what's stored.
	nowMillis := toMillisSinceEpoch(time.Now())
	now := fromMillisSinceEpoch(nowMillis)

	newTree := proto.Clone(tree).(*trillian.Tree)
	newTree.TreeId = id
	newTree.CreateTime = timestamppb.New(now)
	if err := newTree.CreateTime.CheckValid(); err != nil {
		return nil, fmt.Errorf("failed to build create time: %w", err)
	}
	newTree.UpdateTime = timestamppb.New(now)
	if err := newTree.UpdateTime.CheckValid(); err != nil {
		return nil, fmt.Errorf("failed to build update time: %w", err)
	}
	if err := newTree.MaxRootDuration.CheckValid(); err != nil {
		return nil, fmt.Errorf("could not parse MaxRootDuration: %w", err)
	}
	rootDuration := newTree.MaxRootDuration.AsDuration()

	_, err = t.tx.Exec(
		ctx,
		"INSERT INTO Trees(TreeId,TreeState,TreeType,DisplayName,Description,CreateTimeMillis,UpdateTimeMillis,MaxRootDurationMillis) VALUES($1,$2,$3,$4,$5,$6,$7,$8)",
		newTree.TreeId,
		newTree.TreeState.String(),
		newTree.TreeType.String(),
		newTree.DisplayName,
		newTree.Description,
		nowMillis,
		nowMillis,
		rootDuration/time.Millisecond,
	)
	if err != nil {
		return nil, err
	}

	_, err = t.tx.Exec(
		ctx,
		"INSERT INTO TreeControl(TreeId,SigningEnabled,SequencingEnabled,SequenceIntervalSeconds) VALUES($1,$2,$3,$4)",
		newTree.TreeId,
		true, /* SigningEnabled */
		true, /* SequencingEnabled */
		defaultSequenceIntervalSeconds,
	)
	if err != nil {
		return nil, err
	}

	return newTree, nil
}

func (t *adminTX) UpdateTree(ctx context.Context, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, error) {
	tree, err := t.GetTree(ctx, treeID)
	if err != nil {
		return nil, err
	}

	beforeUpdate := proto.Clone(tree).(*trillian.Tree)
	updateFunc(tree)
	if err := storage.ValidateTreeForUpdate(ctx, beforeUpdate, tree); err != nil {
		return nil, err
	}

	// TODO(robstradling): When switching TreeType from PREORDERED_LOG to LOG,
	// ensure all entries in SequencedLeafData are integrated.

	// Use the time truncated-to-millis throughout, as that's what's stored.
	nowMillis := toMillisSinceEpoch(time.Now())
	now := fromMillisSinceEpoch(nowMillis)
	tree.UpdateTime = timestamppb.New(now)
	if err := tree.MaxRootDuration.CheckValid(); err != nil {
		return nil, fmt.Errorf("could not parse MaxRootDuration: %w", err)
	}
	rootDuration := tree.MaxRootDuration.AsDuration()

	if _, err = t.tx.Exec(
		ctx,
		updateTreeSQL,
		tree.TreeState.String(),
		tree.TreeType.String(),
		tree.DisplayName,
		tree.Description,
		nowMillis,
		rootDuration/time.Millisecond,
		tree.TreeId); err != nil {
		return nil, err
	}

	return tree, nil
}

func (t *adminTX) SoftDeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	return t.updateDeleted(ctx, treeID, true /* deleted */, toMillisSinceEpoch(time.Now()) /* deleteTimeMillis */)
}

func (t *adminTX) UndeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	return t.updateDeleted(ctx, treeID, false /* deleted */, nil /* deleteTimeMillis */)
}

// updateDeleted updates the Deleted and DeleteTimeMillis fields of the specified tree.
// deleteTimeMillis must be either an int64 (in millis since epoch) or nil.
func (t *adminTX) updateDeleted(ctx context.Context, treeID int64, deleted bool, deleteTimeMillis interface{}) (*trillian.Tree, error) {
	if err := validateDeleted(ctx, t.tx, treeID, !deleted); err != nil {
		return nil, err
	}
	if _, err := t.tx.Exec(
		ctx,
		"UPDATE Trees SET Deleted=$1, DeleteTimeMillis=$2 WHERE TreeId=$3",
		deleted, deleteTimeMillis, treeID); err != nil {
		return nil, err
	}
	return t.GetTree(ctx, treeID)
}

func (t *adminTX) HardDeleteTree(ctx context.Context, treeID int64) error {
	if err := validateDeleted(ctx, t.tx, treeID, true /* wantDeleted */); err != nil {
		return err
	}

	_, err := t.tx.Exec(ctx, "DELETE FROM Trees WHERE TreeId=$1", treeID)
	return err
}

func validateDeleted(ctx context.Context, tx pgx.Tx, treeID int64, wantDeleted bool) error {
	var nullDeleted sql.NullBool
	switch err := tx.QueryRow(ctx, "SELECT Deleted FROM Trees WHERE TreeId=$1", treeID).Scan(&nullDeleted); {
	case err == pgx.ErrNoRows:
		return status.Errorf(codes.NotFound, "tree %v not found", treeID)
	case err != nil:
		return err
	}

	switch deleted := nullDeleted.Valid && nullDeleted.Bool; {
	case wantDeleted && !deleted:
		return status.Errorf(codes.FailedPrecondition, "tree %v is not soft deleted", treeID)
	case !wantDeleted && deleted:
		return status.Errorf(codes.FailedPrecondition, "tree %v already soft deleted", treeID)
	}
	return nil
}

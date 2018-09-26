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

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultSequenceIntervalSeconds = 60

	nonDeletedWhere = " WHERE (Deleted IS NULL OR Deleted = 'false')"

	selectTreeIDs           = "SELECT TreeId FROM Trees"
	selectNonDeletedTreeIDs = selectTreeIDs + nonDeletedWhere

	selectTrees = `
		SELECT
			TreeId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DisplayName,
			Description,
			CreateTimeMillis,
			UpdateTimeMillis,
			PrivateKey,
			PublicKey,
			MaxRootDurationMillis,
			Deleted,
			DeleteTimeMillis
		FROM Trees`
	selectNonDeletedTrees = selectTrees + nonDeletedWhere
	selectTreeByID        = selectTrees + " WHERE TreeId = ?"

	updateTreeSQL = `UPDATE Trees
		SET TreeState = ?, TreeType = ?, DisplayName = ?, Description = ?, UpdateTimeMillis = ?, MaxRootDurationMillis = ?, PrivateKey = ?
		WHERE TreeId = ?`
)

// NewAdminStorage returns a MySQL storage.AdminStorage implementation backed by DB.
func NewAdminStorage(db *sql.DB) storage.AdminStorage {
	return &mysqlAdminStorage{db}
}

// mysqlAdminStorage implements storage.AdminStorage
type mysqlAdminStorage struct {
	db *sql.DB
}

func (s *mysqlAdminStorage) Snapshot(ctx context.Context) (storage.ReadOnlyAdminTX, error) {
	return s.beginInternal(ctx)
}

func (s *mysqlAdminStorage) beginInternal(ctx context.Context) (storage.AdminTX, error) {
	tx, err := s.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		return nil, err
	}
	return &adminTX{tx: tx}, nil
}

func (s *mysqlAdminStorage) ReadWriteTransaction(ctx context.Context, f storage.AdminTXFunc) error {
	tx, err := s.beginInternal(ctx)
	if err != nil {
		return err
	}
	defer tx.Close()
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *mysqlAdminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

type adminTX struct {
	tx *sql.Tx

	// mu guards *direct* reads/writes on closed, which happen only on
	// Commit/Rollback/IsClosed/Close methods.
	// We don't check closed on *all* methods (apart from the ones above),
	// as we trust tx to keep tabs on its state (and consequently fail to do
	// queries after closed).
	mu     sync.RWMutex
	closed bool
}

func (t *adminTX) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.tx.Commit()
}

func (t *adminTX) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.tx.Rollback()
}

func (t *adminTX) IsClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}

func (t *adminTX) Close() error {
	// Acquire and release read lock manually, without defer, as if the txn
	// is not closed Rollback() will attempt to acquire the rw lock.
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if !closed {
		err := t.Rollback()
		if err != nil {
			glog.Warningf("Rollback error on Close(): %v", err)
		}
		return err
	}
	return nil
}

func (t *adminTX) GetTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	stmt, err := t.tx.PrepareContext(ctx, selectTreeByID)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	// GetTree is an entry point for most RPCs, let's provide somewhat nicer error messages.
	tree, err := storage.ReadTree(stmt.QueryRowContext(ctx, treeID))
	switch {
	case err == sql.ErrNoRows:
		// ErrNoRows doesn't provide useful information, so we don't forward it.
		return nil, status.Errorf(codes.NotFound, "tree %v not found", treeID)
	case err != nil:
		return nil, fmt.Errorf("error reading tree %v: %v", treeID, err)
	}
	return tree, nil
}

func (t *adminTX) ListTreeIDs(ctx context.Context, includeDeleted bool) ([]int64, error) {
	var query string
	if includeDeleted {
		query = selectTreeIDs
	} else {
		query = selectNonDeletedTreeIDs
	}

	stmt, err := t.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	treeIDs := []int64{}
	var treeID int64
	for rows.Next() {
		if err := rows.Scan(&treeID); err != nil {
			return nil, err
		}
		treeIDs = append(treeIDs, treeID)
	}
	return treeIDs, nil
}

func (t *adminTX) ListTrees(ctx context.Context, includeDeleted bool) ([]*trillian.Tree, error) {
	var query string
	if includeDeleted {
		query = selectTrees
	} else {
		query = selectNonDeletedTrees
	}

	stmt, err := t.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	trees := []*trillian.Tree{}
	for rows.Next() {
		tree, err := storage.ReadTree(rows)
		if err != nil {
			return nil, err
		}
		trees = append(trees, tree)
	}
	return trees, nil
}

func (t *adminTX) CreateTree(ctx context.Context, tree *trillian.Tree) (*trillian.Tree, error) {
	if err := storage.ValidateTreeForCreation(ctx, tree); err != nil {
		return nil, err
	}
	if err := validateStorageSettings(tree); err != nil {
		return nil, err
	}

	id, err := storage.NewTreeID()
	if err != nil {
		return nil, err
	}

	// Use the time truncated-to-millis throughout, as that's what's stored.
	nowMillis := storage.ToMillisSinceEpoch(time.Now())
	now := storage.FromMillisSinceEpoch(nowMillis)

	newTree := *tree
	newTree.TreeId = id
	newTree.CreateTime, err = ptypes.TimestampProto(now)
	if err != nil {
		return nil, fmt.Errorf("failed to build create time: %v", err)
	}
	newTree.UpdateTime, err = ptypes.TimestampProto(now)
	if err != nil {
		return nil, fmt.Errorf("failed to build update time: %v", err)
	}
	rootDuration, err := ptypes.Duration(newTree.MaxRootDuration)
	if err != nil {
		return nil, fmt.Errorf("could not parse MaxRootDuration: %v", err)
	}

	insertTreeStmt, err := t.tx.PrepareContext(
		ctx,
		`INSERT INTO Trees(
			TreeId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DisplayName,
			Description,
			CreateTimeMillis,
			UpdateTimeMillis,
			PrivateKey,
			PublicKey,
			MaxRootDurationMillis)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insertTreeStmt.Close()

	privateKey, err := proto.Marshal(newTree.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("could not marshal PrivateKey: %v", err)
	}

	_, err = insertTreeStmt.ExecContext(
		ctx,
		newTree.TreeId,
		newTree.TreeState.String(),
		newTree.TreeType.String(),
		newTree.HashStrategy.String(),
		newTree.HashAlgorithm.String(),
		newTree.SignatureAlgorithm.String(),
		newTree.DisplayName,
		newTree.Description,
		nowMillis,
		nowMillis,
		privateKey,
		newTree.PublicKey.GetDer(),
		rootDuration/time.Millisecond,
	)
	if err != nil {
		return nil, err
	}

	// MySQL silently truncates data when running in non-strict mode.
	// We shouldn't be using non-strict modes, but let's guard against it
	// anyway.
	if _, err := t.GetTree(ctx, newTree.TreeId); err != nil {
		// GetTree will fail for truncated enums (they get recorded as
		// empty strings, which will not match any known value).
		return nil, fmt.Errorf("enum truncated: %v", err)
	}

	insertControlStmt, err := t.tx.PrepareContext(
		ctx,
		`INSERT INTO TreeControl(
			TreeId,
			SigningEnabled,
			SequencingEnabled,
			SequenceIntervalSeconds)
		VALUES(?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insertControlStmt.Close()
	_, err = insertControlStmt.ExecContext(
		ctx,
		newTree.TreeId,
		true, /* SigningEnabled */
		true, /* SequencingEnabled */
		defaultSequenceIntervalSeconds,
	)
	if err != nil {
		return nil, err
	}

	return &newTree, nil
}

func (t *adminTX) UpdateTree(ctx context.Context, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, error) {
	tree, err := t.GetTree(ctx, treeID)
	if err != nil {
		return nil, err
	}

	beforeUpdate := *tree
	updateFunc(tree)
	if err := storage.ValidateTreeForUpdate(ctx, &beforeUpdate, tree); err != nil {
		return nil, err
	}
	if err := validateStorageSettings(tree); err != nil {
		return nil, err
	}

	// TODO(pavelkalinnikov): When switching TreeType from PREORDERED_LOG to LOG,
	// ensure all entries in SequencedLeafData are integrated.

	// Use the time truncated-to-millis throughout, as that's what's stored.
	nowMillis := storage.ToMillisSinceEpoch(time.Now())
	now := storage.FromMillisSinceEpoch(nowMillis)
	tree.UpdateTime, err = ptypes.TimestampProto(now)
	if err != nil {
		return nil, fmt.Errorf("failed to build update time: %v", err)
	}
	rootDuration, err := ptypes.Duration(tree.MaxRootDuration)
	if err != nil {
		return nil, fmt.Errorf("could not parse MaxRootDuration: %v", err)
	}

	privateKey, err := proto.Marshal(tree.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("could not marshal PrivateKey: %v", err)
	}

	stmt, err := t.tx.PrepareContext(ctx, updateTreeSQL)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if _, err = stmt.ExecContext(
		ctx,
		tree.TreeState.String(),
		tree.TreeType.String(),
		tree.DisplayName,
		tree.Description,
		nowMillis,
		rootDuration/time.Millisecond,
		privateKey,
		tree.TreeId); err != nil {
		return nil, err
	}

	return tree, nil
}

func (t *adminTX) SoftDeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	return t.updateDeleted(ctx, treeID, true /* deleted */, storage.ToMillisSinceEpoch(time.Now()) /* deleteTimeMillis */)
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
	if _, err := t.tx.ExecContext(
		ctx,
		"UPDATE Trees SET Deleted = ?, DeleteTimeMillis = ? WHERE TreeId = ?",
		deleted, deleteTimeMillis, treeID); err != nil {
		return nil, err
	}
	return t.GetTree(ctx, treeID)
}

func (t *adminTX) HardDeleteTree(ctx context.Context, treeID int64) error {
	if err := validateDeleted(ctx, t.tx, treeID, true /* wantDeleted */); err != nil {
		return err
	}

	// TreeControl didn't have "ON DELETE CASCADE" on previous versions, so let's hit it explicitly
	if _, err := t.tx.ExecContext(ctx, "DELETE FROM TreeControl WHERE TreeId = ?", treeID); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(ctx, "DELETE FROM Trees WHERE TreeId = ?", treeID)
	return err
}

func validateDeleted(ctx context.Context, tx *sql.Tx, treeID int64, wantDeleted bool) error {
	var nullDeleted sql.NullBool
	switch err := tx.QueryRowContext(ctx, "SELECT Deleted FROM Trees WHERE TreeId = ?", treeID).Scan(&nullDeleted); {
	case err == sql.ErrNoRows:
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

func validateStorageSettings(tree *trillian.Tree) error {
	if tree.StorageSettings != nil {
		return fmt.Errorf("storage_settings not supported, but got %v", tree.StorageSettings)
	}
	return nil
}

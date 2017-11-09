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
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/errors"
	"github.com/google/trillian/storage"
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
	return s.Begin(ctx)
}

func (s *mysqlAdminStorage) Begin(ctx context.Context) (storage.AdminTX, error) {
	tx, err := s.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		return nil, err
	}
	return &adminTX{tx: tx}, nil
}

func (s *mysqlAdminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, s.db)
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
	tree, err := readTree(stmt.QueryRowContext(ctx, treeID))
	switch {
	case err == sql.ErrNoRows:
		// ErrNoRows doesn't provide useful information, so we don't forward it.
		return nil, errors.Errorf(errors.NotFound, "tree %v not found", treeID)
	case err != nil:
		return nil, fmt.Errorf("error reading tree %v: %v", treeID, err)
	}
	return tree, nil
}

// There's no common interface between sql.Row and sql.Rows(!), so we have to
// define one.
type row interface {
	Scan(dest ...interface{}) error
}

func readTree(row row) (*trillian.Tree, error) {
	tree := &trillian.Tree{}

	// Enums and Datetimes need an extra conversion step
	var treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm string
	var createMillis, updateMillis, maxRootDurationMillis int64
	var displayName, description sql.NullString
	var privateKey, publicKey []byte
	var deleted sql.NullBool
	var deleteMillis sql.NullInt64
	err := row.Scan(
		&tree.TreeId,
		&treeState,
		&treeType,
		&hashStrategy,
		&hashAlgorithm,
		&signatureAlgorithm,
		&displayName,
		&description,
		&createMillis,
		&updateMillis,
		&privateKey,
		&publicKey,
		&maxRootDurationMillis,
		&deleted,
		&deleteMillis,
	)
	if err != nil {
		return nil, err
	}

	setNullStringIfValid(displayName, &tree.DisplayName)
	setNullStringIfValid(description, &tree.Description)

	// Convert all things!
	if ts, ok := trillian.TreeState_value[treeState]; ok {
		tree.TreeState = trillian.TreeState(ts)
	} else {
		return nil, fmt.Errorf("unknown TreeState: %v", treeState)
	}
	if tt, ok := trillian.TreeType_value[treeType]; ok {
		tree.TreeType = trillian.TreeType(tt)
	} else {
		return nil, fmt.Errorf("unknown TreeType: %v", treeType)
	}
	if hs, ok := trillian.HashStrategy_value[hashStrategy]; ok {
		tree.HashStrategy = trillian.HashStrategy(hs)
	} else {
		return nil, fmt.Errorf("unknown HashStrategy: %v", hashStrategy)
	}
	if ha, ok := spb.DigitallySigned_HashAlgorithm_value[hashAlgorithm]; ok {
		tree.HashAlgorithm = spb.DigitallySigned_HashAlgorithm(ha)
	} else {
		return nil, fmt.Errorf("unknown HashAlgorithm: %v", hashAlgorithm)
	}
	if sa, ok := spb.DigitallySigned_SignatureAlgorithm_value[signatureAlgorithm]; ok {
		tree.SignatureAlgorithm = spb.DigitallySigned_SignatureAlgorithm(sa)
	} else {
		return nil, fmt.Errorf("unknown SignatureAlgorithm: %v", signatureAlgorithm)
	}

	// Let's make sure we didn't mismatch any of the casts above
	ok := tree.TreeState.String() == treeState
	ok = ok && tree.TreeType.String() == treeType
	ok = ok && tree.HashStrategy.String() == hashStrategy
	ok = ok && tree.HashAlgorithm.String() == hashAlgorithm
	ok = ok && tree.SignatureAlgorithm.String() == signatureAlgorithm
	if !ok {
		return nil, fmt.Errorf(
			"mismatched enum: tree = %v, enums = [%v, %v, %v, %v, %v]",
			tree,
			treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm)
	}

	tree.CreateTime, err = ptypes.TimestampProto(fromMillisSinceEpoch(createMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse create time: %v", err)
	}
	tree.UpdateTime, err = ptypes.TimestampProto(fromMillisSinceEpoch(updateMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse update time: %v", err)
	}
	tree.MaxRootDuration = ptypes.DurationProto(time.Duration(maxRootDurationMillis * int64(time.Millisecond)))

	tree.PrivateKey = &any.Any{}
	if err := proto.Unmarshal(privateKey, tree.PrivateKey); err != nil {
		return nil, fmt.Errorf("could not unmarshal PrivateKey: %v", err)
	}
	tree.PublicKey = &keyspb.PublicKey{Der: publicKey}

	tree.Deleted = deleted.Valid && deleted.Bool
	if tree.Deleted && deleteMillis.Valid {
		tree.DeleteTime, err = ptypes.TimestampProto(fromMillisSinceEpoch(deleteMillis.Int64))
		if err != nil {
			return nil, fmt.Errorf("failed to parse delete time: %v", err)
		}
	}

	return tree, nil
}

// setNullStringIfValid assigns src to dest if src is Valid.
func setNullStringIfValid(src sql.NullString, dest *string) {
	if src.Valid {
		*dest = src.String
	}
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
		tree, err := readTree(rows)
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
	nowMillis := toMillisSinceEpoch(time.Now())
	now := fromMillisSinceEpoch(nowMillis)

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

	// TODO(codingllama): There's a strong disconnect between trillian.Tree and TreeControl. Are we OK with that?
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

	// Use the time truncated-to-millis throughout, as that's what's stored.
	nowMillis := toMillisSinceEpoch(time.Now())
	now := fromMillisSinceEpoch(nowMillis)
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

	stmt, err := t.tx.PrepareContext(
		ctx,
		`UPDATE Trees
		SET TreeState = ?, DisplayName = ?, Description = ?, UpdateTimeMillis = ?, MaxRootDurationMillis = ?, PrivateKey = ?
		WHERE TreeId = ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if _, err = stmt.ExecContext(
		ctx,
		tree.TreeState.String(),
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
		return errors.Errorf(errors.NotFound, "tree %v not found", treeID)
	case err != nil:
		return err
	}

	switch deleted := nullDeleted.Valid && nullDeleted.Bool; {
	case wantDeleted && !deleted:
		return errors.Errorf(errors.FailedPrecondition, "tree %v is not soft deleted", treeID)
	case !wantDeleted && deleted:
		return errors.Errorf(errors.FailedPrecondition, "tree %v already soft deleted", treeID)
	}
	return nil
}

func toMillisSinceEpoch(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

func fromMillisSinceEpoch(ts int64) time.Time {
	secs := int64(ts / 1000)
	msecs := int64(ts % 1000)
	return time.Unix(secs, msecs*1000000)
}

func validateStorageSettings(tree *trillian.Tree) error {
	if tree.StorageSettings != nil {
		return fmt.Errorf("storage_settings not supported, but got %v", tree.StorageSettings)
	}
	return nil
}

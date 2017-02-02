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
	"database/sql"
	"fmt"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
	"sync"
	"time"
)

const (
	defaultSequenceIntervalSeconds = 60
	defaultSignIntervalSeconds     = 60
)

var (
	selectTrees = `
		SELECT
			TreeId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DuplicatePolicy,
			DisplayName,
			Description,
			CreateTime,
			UpdateTime
		FROM Trees`
	selectTreeByID = selectTrees + " WHERE TreeId = ?"
)

// duplicatePolicyMap maps storage enums to trillian.DuplicatePolicy enums,
// which differ slightly.
var duplicatePolicyMap = map[string]trillian.DuplicatePolicy{
	"NOT_ALLOWED": trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED,
	"ALLOWED":     trillian.DuplicatePolicy_DUPLICATES_ALLOWED,
}

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
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &adminTX{tx: tx}, nil
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
		return t.Rollback()
	}
	return nil
}

func (t *adminTX) GetTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	stmt, err := t.tx.Prepare(selectTreeByID)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return readTree(stmt.QueryRow(treeID))
}

// There's no common interface between sql.Row and sql.Rows(!), so we have to
// define one.
type row interface {
	Scan(dest ...interface{}) error
}

func readTree(row row) (*trillian.Tree, error) {
	tree := &trillian.Tree{}

	// Enums and Datetimes need an extra conversion step
	var treeState string
	var treeType string
	var hashStrategy string
	var hashAlgorithm string
	var signatureAlgorithm string
	var duplicatePolicy string
	var createDatetime string
	var updateDatetime string

	err := row.Scan(
		&tree.TreeId,
		&treeState,
		&treeType,
		&hashStrategy,
		&hashAlgorithm,
		&signatureAlgorithm,
		&duplicatePolicy,
		&tree.DisplayName,
		&tree.Description,
		&createDatetime,
		&updateDatetime,
	)
	if err != nil {
		return nil, err
	}

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
	if ha, ok := trillian.HashAlgorithm_value[hashAlgorithm]; ok {
		tree.HashAlgorithm = trillian.HashAlgorithm(ha)
	} else {
		return nil, fmt.Errorf("unknown HashAlgorithm: %v", hashAlgorithm)
	}
	if sa, ok := trillian.SignatureAlgorithm_value[signatureAlgorithm]; ok {
		tree.SignatureAlgorithm = trillian.SignatureAlgorithm(sa)
	} else {
		return nil, fmt.Errorf("unknown SignatureAlgorithm: %v", signatureAlgorithm)
	}
	// Slightly different from the ones above, as duplicatePolicyMap is a map we maintain.
	// That's because DuplicatePolicy values don't exactly match storage enums.
	if dp, ok := duplicatePolicyMap[duplicatePolicy]; ok {
		tree.DuplicatePolicy = dp
	} else {
		return nil, fmt.Errorf("unknown DuplicatePolicy: %v", duplicatePolicy)
	}

	// Let's make sure we didn't mismatch any of the casts above
	ok := tree.TreeState.String() == treeState
	ok = ok && tree.TreeType.String() == treeType
	ok = ok && tree.HashStrategy.String() == hashStrategy
	ok = ok && tree.HashAlgorithm.String() == hashAlgorithm
	ok = ok && tree.SignatureAlgorithm.String() == signatureAlgorithm
	ok = ok && tree.DuplicatePolicy == duplicatePolicyMap[duplicatePolicy]
	if !ok {
		return nil, fmt.Errorf(
			"mismatched enum: tree = %v, enums = [%v, %v, %v, %v, %v, %v]",
			tree,
			treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm, duplicatePolicy)
	}

	createTime, err := parseDatetime(createDatetime)
	if err != nil {
		return nil, err
	}
	tree.CreateTimeMillisSinceEpoch = toMillisSinceEpoch(createTime)

	updateTime, err := parseDatetime(updateDatetime)
	if err != nil {
		return nil, err
	}
	tree.UpdateTimeMillisSinceEpoch = toMillisSinceEpoch(updateTime)

	return tree, nil
}

func (t *adminTX) ListTreeIDs(ctx context.Context) ([]int64, error) {
	stmt, err := t.tx.Prepare("SELECT TreeId FROM Trees")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query()
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

func (t *adminTX) ListTrees(ctx context.Context) ([]*trillian.Tree, error) {
	stmt, err := t.tx.Prepare(selectTrees)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query()
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
	if err := storage.ValidateTreeForCreation(tree); err != nil {
		return nil, err
	}

	id, err := storage.NewTreeID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	nowDatetime := toDatetime(now)
	nowMillis := toMillisSinceEpoch(now)

	newTree := *tree
	newTree.TreeId = id
	newTree.CreateTimeMillisSinceEpoch = nowMillis
	newTree.UpdateTimeMillisSinceEpoch = nowMillis

	insertTreeStmt, err := t.tx.Prepare(`
		INSERT INTO Trees(
			TreeId,
			KeyId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DuplicatePolicy,
			DisplayName,
			Description,
			CreateTime,
			UpdateTime,
			LeafHasherType,
			TreeHasherType,
			AllowsDuplicateLeaves)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insertTreeStmt.Close()

	// DuplicatePolicy doesn't map exactly to the enum, search in the map
	// instead.
	duplicatePolicy := ""
	for k, v := range duplicatePolicyMap {
		if v == newTree.DuplicatePolicy {
			duplicatePolicy = k
			break
		}
	}
	if duplicatePolicy == "" {
		return nil, fmt.Errorf("unexpected DuplicatePolicy value: %v", newTree.DuplicatePolicy)
	}

	_, err = insertTreeStmt.Exec(
		newTree.TreeId,
		1, /* KeyId */
		newTree.TreeState.String(),
		newTree.TreeType.String(),
		newTree.HashStrategy.String(),
		newTree.HashAlgorithm.String(),
		newTree.SignatureAlgorithm.String(),
		duplicatePolicy,
		newTree.DisplayName,
		newTree.Description,
		nowDatetime,                                                            /* CreateTime */
		nowDatetime,                                                            /* UpdateTime */
		newTree.HashAlgorithm.String(),                                         /* LeafHasherType */
		newTree.HashAlgorithm.String(),                                         /* TreeHasherType */
		newTree.DuplicatePolicy == trillian.DuplicatePolicy_DUPLICATES_ALLOWED, /* AllowsDuplicateLeaves */
	)
	if err != nil {
		return nil, err
	}

	// MySQL silently truncates data when running on non-strict mode.
	// We shouldn't be using non-strict modes, but let's guard against it
	// anyway.
	if _, err := t.GetTree(ctx, newTree.TreeId); err != nil {
		// GetTree will fail for truncated enums (they get recorded as
		// empty strings, which will not match any known value).
		return nil, fmt.Errorf("enum truncated: %v", err)
	}

	// TODO(codingllama): There's a strong disconnect between trillian.Tree and TreeControl. Are we OK with that?
	insertControlStmt, err := t.tx.Prepare(`
		INSERT INTO TreeControl(
			TreeId,
			ReadOnlyRequests,
			SigningEnabled,
			SequencingEnabled,
			SequenceIntervalSeconds,
			SignIntervalSeconds)
		VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insertControlStmt.Close()
	_, err = insertControlStmt.Exec(
		newTree.TreeId,
		false, /* ReadOnlyRequests */
		true,  /* SigningEnabled */
		true,  /* SequencingEnabled */
		defaultSequenceIntervalSeconds,
		defaultSignIntervalSeconds,
	)
	if err != nil {
		return nil, err
	}

	return &newTree, nil
}

func toMillisSinceEpoch(t time.Time) int64 {
	// Don't bother with UnixNano(), MySQL only stores second-precision
	return t.Unix() * 1000
}

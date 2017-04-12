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

// Package wrapper defines a wrapping mechanism for databases and decouples the logic
// of accessing the database from the creation of statements in a specific SQL format.
// It's strongly related to the coresql package but cannot be packaged with it because it's
// then impossible to avoid import cycles.
package wrapper

import (
	"context"
	"database/sql"
)

// TreeStatementProvider provides SQL statement objects for raw tree storage.
type TreeStatementProvider interface {
	GetTreeRevisionIncludingSizeStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetSubtreeStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
	SetSubtreeStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
}

// LogStatementProvider provides SQL statement objects for log storage.
type LogStatementProvider interface {
	GetActiveLogsStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetActiveLogsWithWorkStmt(tx *sql.Tx) (*sql.Stmt, error)
	DeleteUnsequencedStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
	GetLeavesByIndexStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
	GetLeavesByMerkleHashStmt(tx *sql.Tx, num int, orderBySequence bool) (*sql.Stmt, error)
	GetLeavesByLeafIdentityHashStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
	InsertTreeHeadStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetLatestSignedLogRootStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetQueuedLeavesStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertUnsequencedEntryStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertUnsequencedLeafStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertSequencedLeafStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetSequencedLeafCountStmt(tx *sql.Tx) (*sql.Stmt, error)
}

// MapStatementProvider provides SQL statement objects for map storage.
type MapStatementProvider interface {
	GetMapLeafStmt(tx *sql.Tx, num int) (*sql.Stmt, error)
	GetLatestMapRootStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertMapHeadStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertMapLeafStmt(tx *sql.Tx) (*sql.Stmt, error)
}

// AdminStatementProvider provides SQL statement objects for administration.
type AdminStatementProvider interface {
	GetTreeIDsStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetAllTreesStmt(tx *sql.Tx) (*sql.Stmt, error)
	GetTreeStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertTreeStmt(tx *sql.Tx) (*sql.Stmt, error)
	InsertTreeControlStmt(tx *sql.Tx) (*sql.Stmt, error)
	UpdateTreeStmt(tx *sql.Tx) (*sql.Stmt, error)
}

// CustomBehaviourProvider abstracts database specific features, for example error code checking.
type CustomBehaviourProvider interface {
	CheckDatabaseAccessible(ctx context.Context) error
	IsDuplicateErr(err error) bool
	TreeRowExists(treeID int64) error
}

// LifecycleHooks allows implementations to add custom logic at various points in the
// database and transaction flow.
type LifecycleHooks interface {
	OnOpenDB() error
}

// DBWrapper encapsulates a database and provides customized SQL statement objects for all types
// of tree storage as well as abstracting some operations that differ between databases.
// Statements returned by any of public functions belong to the caller transaction and must be
// closed on completion of the work.
type DBWrapper interface {
	TreeStatementProvider
	LogStatementProvider
	MapStatementProvider
	AdminStatementProvider
	LifecycleHooks
	CustomBehaviourProvider
	DB() *sql.DB
}

// GetStmtFunc is a function that creates and returns a pointer to a sql.Stmt and an error.
type GetStmtFunc func() (stmt *sql.Stmt, err error)

// PrepInTx indirectly obtains a pointer to a SQL statement via a supplied function and if
// this succeeds returns a prepared statement in the given transaction that is owned by the
// caller.
func PrepInTx(tx *sql.Tx, fn GetStmtFunc) (*sql.Stmt, error) {
	stmt, err := fn()
	if err != nil {
		return nil, err
	}
	return tx.Stmt(stmt), nil
}

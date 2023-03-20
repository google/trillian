// Package stmtcache contains tools for managing the prepared-statement cache.
package stmtcache

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/google/trillian/monitoring"
	"k8s.io/klog/v2"
)

var (
	once           sync.Once
	errStmtCounter monitoring.Counter
)

// PlaceholderSQL SQL statement placeholder
const PlaceholderSQL = "<placeholder>"

// Stmt is wraps the sql.Stmt struct for handling and monitoring SQL errors.
// If Stmt execution errors occur, it is automatically closed and the prepared statements in the cache are cleared.
type Stmt struct {
	statement      string
	placeholderNum int
	stmtCache      *StmtCache
	stmt           *sql.Stmt
	parentStmt     *Stmt
}

// errHandler handling and monitoring SQL errors
// The err parameter standby
func (s *Stmt) errHandler(_ error) {
	o := s
	if s.parentStmt != nil {
		o = s.parentStmt
	}

	if err := o.Close(); err != nil {
		klog.Warningf("Failed to close stmt: %s", err)
	}

	if o.stmtCache != nil {
		once.Do(func() {
			errStmtCounter = o.stmtCache.mf.NewCounter("sql_stmt_errors", "Number of statement execution errors")
		})

		errStmtCounter.Inc()
	}
}

// SQLStmt returns the referenced sql.Stmt struct.
func (s *Stmt) SQLStmt() *sql.Stmt {
	return s.stmt
}

// Close closes the Stmt.
// Clear if Stmt belongs to cache
func (s *Stmt) Close() error {
	if cache := s.stmtCache; cache != nil {
		cache.clearOne(s)
	}

	return s.stmt.Close()
}

// WithTx returns a transaction-specific prepared statement from
// an existing statement.
func (s *Stmt) WithTx(ctx context.Context, tx *sql.Tx) *Stmt {
	parent := s
	if s.parentStmt != nil {
		parent = s.parentStmt
	}
	return &Stmt{
		parentStmt: parent,
		stmt:       tx.StmtContext(ctx, parent.stmt),
	}
}

// Exec executes a prepared statement with the given arguments and
// returns a Result summarizing the effect of the statement.
//
// Exec uses context.Background internally; to specify the context, use
// ExecContext.
func (s *Stmt) Exec(args ...interface{}) (sql.Result, error) {
	res, err := s.stmt.Exec(args...)
	if err != nil {
		s.errHandler(err)
	}
	return res, err
}

// ExecContext executes a prepared statement with the given arguments and
// returns a Result summarizing the effect of the statement.
func (s *Stmt) ExecContext(ctx context.Context, args ...interface{}) (sql.Result, error) {
	res, err := s.stmt.ExecContext(ctx, args...)
	if err != nil {
		s.errHandler(err)
	}
	return res, err
}

// Query executes a prepared query statement with the given arguments
// and returns the query results as a *Rows.
//
// Query uses context.Background internally; to specify the context, use
// QueryContext.
func (s *Stmt) Query(args ...interface{}) (*sql.Rows, error) {
	res, err := s.stmt.Query(args...)
	if err != nil {
		s.errHandler(err)
	}
	return res, err
}

// QueryContext executes a prepared query statement with the given arguments
// and returns the query results as a *Rows.
func (s *Stmt) QueryContext(ctx context.Context, args ...interface{}) (*sql.Rows, error) {
	res, err := s.stmt.QueryContext(ctx, args...)
	if err != nil {
		s.errHandler(err)
	}
	return res, err
}

// QueryRow executes a prepared query statement with the given arguments.
// If an error occurs during the execution of the statement, that error will
// be returned by a call to Scan on the returned *Row, which is always non-nil.
// If the query selects no rows, the *Row's Scan will return ErrNoRows.
// Otherwise, the *Row's Scan scans the first selected row and discards
// the rest.
//
// Example usage:
//
//	var name string
//	err := nameByUseridStmt.QueryRow(id).Scan(&name)
//
// QueryRow uses context.Background internally; to specify the context, use
// QueryRowContext.
func (s *Stmt) QueryRow(args ...interface{}) *sql.Row {
	res := s.stmt.QueryRow(args...)
	if err := res.Err(); err != nil {
		s.errHandler(err)
	}
	return res
}

// QueryRowContext executes a prepared query statement with the given arguments.
// If an error occurs during the execution of the statement, that error will
// be returned by a call to Scan on the returned *Row, which is always non-nil.
// If the query selects no rows, the *Row's Scan will return ErrNoRows.
// Otherwise, the *Row's Scan scans the first selected row and discards
// the rest.
func (s *Stmt) QueryRowContext(ctx context.Context, args ...any) *sql.Row {
	res := s.stmt.QueryRowContext(ctx, args...)
	if err := res.Err(); err != nil {
		s.errHandler(err)
	}
	return res
}

// StmtCache is a cache of the sql.Stmt structs.
type StmtCache struct {
	db             *sql.DB
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
	mf             monitoring.MetricFactory
}

// NewStmtCache creates a StmtCache instance.
func NewStmtCache(db *sql.DB, mf monitoring.MetricFactory) *StmtCache {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}

	return &StmtCache{
		db:         db,
		statements: make(map[string]map[int]*sql.Stmt),
		mf:         mf,
	}
}

// clearOne clear the cache of a sql.Stmt.
func (sc *StmtCache) clearOne(s *Stmt) {
	if s == nil || s.stmt == nil || s.stmtCache != sc {
		return
	}

	sc.statementMutex.Lock()
	defer sc.statementMutex.Unlock()

	if sc.statements[s.statement] != nil && sc.statements[s.statement][s.placeholderNum] == s.stmt {
		sc.statements[s.statement][s.placeholderNum] = nil
	}
}

func (sc *StmtCache) getStmt(ctx context.Context, statement string, num int, first, rest string) (*sql.Stmt, error) {
	sc.statementMutex.Lock()
	defer sc.statementMutex.Unlock()

	if sc.statements[statement] != nil {
		if sc.statements[statement][num] != nil {
			return sc.statements[statement][num], nil
		}
	} else {
		sc.statements[statement] = make(map[int]*sql.Stmt)
	}

	s, err := sc.db.PrepareContext(ctx, expandPlaceholderSQL(statement, num, first, rest))
	if err != nil {
		klog.Warningf("Failed to prepare statement %d: %s", num, err)
		return nil, err
	}

	sc.statements[statement][num] = s

	return s, nil
}

// expandPlaceholderSQL expands an sql statement by adding a specified number of '?'
// placeholder slots. At most one placeholder will be expanded.
func expandPlaceholderSQL(sql string, num int, first, rest string) string {
	if num <= 0 {
		panic(fmt.Errorf("trying to expand SQL placeholder with <= 0 parameters: %s", sql))
	}

	parameters := first + strings.Repeat(","+rest, num-1)

	return strings.Replace(sql, PlaceholderSQL, parameters, 1)
}

// GetStmt creates and caches sql.Stmt structs and returns their wrapper structs Stmt.
func (sc *StmtCache) GetStmt(ctx context.Context, statement string, num int, first, rest string) (*Stmt, error) {
	stmt, err := sc.getStmt(ctx, statement, num, first, rest)
	if err != nil {
		return nil, err
	}

	return &Stmt{
		statement:      statement,
		placeholderNum: num,
		stmtCache:      sc,
		stmt:           stmt,
	}, nil
}

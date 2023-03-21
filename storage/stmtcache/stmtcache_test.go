package stmtcache_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/trillian/storage/stmtcache"
	"github.com/google/trillian/storage/testdb"
	"k8s.io/klog/v2"
)

var db *sql.DB

func TestMain(m *testing.M) {
	if !testdb.MySQLAvailable() {
		klog.Errorf("MySQL not available, skipping all stmt tests")
		return
	}
	ctx := context.Background()

	var done func(context.Context)
	var err error
	db, done, err = testdb.NewTrillianDB(ctx, testdb.DriverMySQL)
	if err != nil {
		panic(err)
	}

	status := m.Run()
	done(context.Background())
	os.Exit(status)
}

func TestStmtExecContext(t *testing.T) {
	ctx := context.Background()
	cache := stmtcache.New(db, nil)
	sql := `SELECT ` + stmtcache.PlaceholderSQL
	stmt, err := cache.GetStmt(ctx, sql, 1, "?", "")
	if err != nil {
		t.Fatalf("failed to cache.GetStmt: %s", err)
	}
	if _, err = stmt.ExecContext(ctx, ""); err != nil {
		t.Fatalf("failed to stmt.ExecContext: %s", err)
	}
}

func TestStmtQueryContext(t *testing.T) {
	ctx := context.Background()
	cache := stmtcache.New(db, nil)
	sql := `SELECT ` + stmtcache.PlaceholderSQL
	stmt, err := cache.GetStmt(ctx, sql, 1, "?", "")
	if err != nil {
		t.Fatalf("failed to cache.GetStmt: %s", err)
	}
	want := "TestQuery"
	rows, err := stmt.QueryContext(ctx, want)
	if err != nil {
		t.Fatalf("failed to stmt.QueryContext: %s", err)
	}
	defer rows.Close()

	rows.Next()
	var res string
	if err := rows.Scan(&res); err != nil {
		t.Fatalf("failed to rows.Scan: %s", err)
	}

	if res != want {
		t.Errorf("unexpected results")
	}
}

func TestStmtQueryRowContext(t *testing.T) {
	ctx := context.Background()
	cache := stmtcache.New(db, nil)
	sql := `SELECT ` + stmtcache.PlaceholderSQL
	stmt, err := cache.GetStmt(ctx, sql, 1, "?", "")
	if err != nil {
		t.Fatalf("failed to cache.GetStmt: %s", err)
	}
	want := "TestQueryRow"
	row := stmt.QueryRowContext(ctx, want)
	if err != nil {
		t.Fatalf("failed to stmt.QueryRowContext: %s", err)
	}
	var res string
	if err := row.Scan(&res); err != nil {
		t.Fatalf("failed to row.Scan: %s", err)
	}

	if res != want {
		t.Errorf("unexpected results")
	}
}

func TestStmtWithTx(t *testing.T) {
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE TestStmtWithTx(ID int)"); err != nil {
		t.Fatalf("failed to create table: %s", err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DROP TABLE TestStmtWithTx"); err != nil {
			klog.Errorf("failed to drop table: %s", err)
		}
	}()

	cache := stmtcache.New(db, nil)
	sql := `INSERT INTO TestStmtWithTx(ID) ` + stmtcache.PlaceholderSQL
	stmt, err := cache.GetStmt(ctx, sql, 1, "VALUES(?)", "(?)")
	if err != nil {
		t.Fatalf("failed to cache.GetStmt: %s", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to db.BeginTx: %s", err)
	}
	stx := stmt.WithTx(ctx, tx)
	defer stx.Close()

	id := 1
	_, err = stx.ExecContext(ctx, id)
	if err != nil {
		t.Fatalf("failed to stx.ExecContext: %s", err)
	}

	if err := tx.Rollback(); err != nil {
		klog.Errorf("failed to tx.Rollback: %s", err)
	}

	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM TestStmtWithTx WHERE ID = ?", id)
	var count int
	if err = row.Scan(&count); err != nil {
		t.Fatalf("failed to row.Scan: %s", err)
	}

	if count != 0 {
		t.Errorf("transaction not rolled back")
	}
}

// func TestStmtExecutionError(t *testing.T) {
// 	db.SetMaxIdleConns(0)
// 	cache := stmtcache.New(db, nil)
// 	ctx := context.Background()
// 	sql := `SELECT ` + stmtcache.PlaceholderSQL
// 	_, err := cache.GetStmt(ctx, sql, 1, "?", "")
// 	if err != nil {
// 		t.Fatalf("failed to cache.GetStmt: %s", err)
// 	}
// 	row := db.QueryRowContext(ctx, "SELECT sys.ps_thread_id(connection_id())")
// 	var tid int
// 	if err = row.Scan(&tid); err != nil {
// 		t.Fatalf("failed to get sys.ps_thread_id: %s", err)
// 	}
// }

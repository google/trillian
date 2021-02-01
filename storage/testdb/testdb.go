// Copyright 2017 Google LLC. All Rights Reserved.
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

// Package testdb creates new databases for tests.
package testdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/testonly"
	"golang.org/x/sys/unix"

	_ "github.com/go-sql-driver/mysql" // mysql driver
)

const (
	// MySQLURIEnv is the name of the ENV variable checked for the test MySQL
	// instance URI to use. The value must have a trailing slash.
	MySQLURIEnv = "TEST_MYSQL_URI"

	// Note: sql.Open requires the URI to end with a slash.
	defaultTestMySQLURI = "root@tcp(127.0.0.1)/"
)

var trillianSQL = testonly.RelativeToPackage("../mysql/schema/storage.sql")

// mysqlURI returns the MySQL connection URI to use for tests. It returns the
// value in the ENV variable defined by MySQLURIEnv. If the value is empty,
// returns defaultTestMySQLURI.
//
// We use an ENV variable, rather than a flag, for flexibility. Only a subset
// of the tests in this repo require a database and import this package. With a
// flag, it would be necessary to distinguish "go test" invocations that need a
// database, and those that don't. ENV allows to "blanket apply" this setting.
func mysqlURI() string {
	if e := os.Getenv(MySQLURIEnv); len(e) > 0 {
		return e
	}
	return defaultTestMySQLURI
}

// MySQLAvailable indicates whether the configured MySQL database is available.
func MySQLAvailable() bool {
	db, err := sql.Open("mysql", mysqlURI())
	if err != nil {
		log.Printf("sql.Open(): %v", err)
		return false
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Printf("db.Ping(): %v", err)
		return false
	}
	return true
}

// SetFDLimit sets the soft limit on the maximum number of open file descriptors.
// See http://man7.org/linux/man-pages/man2/setrlimit.2.html
func SetFDLimit(uLimit uint64) error {
	var rLimit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	if uLimit > rLimit.Max {
		return fmt.Errorf("Could not set FD limit to %v. Must be less than the hard limit %v", uLimit, rLimit.Max)
	}
	rLimit.Cur = uLimit
	return unix.Setrlimit(unix.RLIMIT_NOFILE, &rLimit)
}

// newEmptyDB creates a new, empty database.
// It returns the database handle and a clean-up function, or an error.
// The returned clean-up function should be called once the caller is finished
// using the DB, the caller should not continue to use the returned DB after
// calling this function as it may, for example, delete the underlying
// instance.
func newEmptyDB(ctx context.Context) (*sql.DB, func(context.Context), error) {
	if err := SetFDLimit(2048); err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("mysql", mysqlURI())
	if err != nil {
		return nil, nil, err
	}

	// Create a randomly-named database and then connect using the new name.
	name := fmt.Sprintf("trl_%v", time.Now().UnixNano())

	stmt := fmt.Sprintf("CREATE DATABASE %v", name)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return nil, nil, fmt.Errorf("error running statement %q: %v", stmt, err)
	}

	db.Close()
	db, err = sql.Open("mysql", mysqlURI()+name)
	if err != nil {
		return nil, nil, err
	}

	done := func(ctx context.Context) {
		defer db.Close()
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %v", name)); err != nil {
			glog.Warningf("Failed to drop test database %q: %v", name, err)
		}
	}

	return db, done, db.Ping()
}

// NewTrillianDB creates an empty database with the Trillian schema. The database name is randomly
// generated.
// NewTrillianDB is equivalent to Default().NewTrillianDB(ctx).
func NewTrillianDB(ctx context.Context) (*sql.DB, func(context.Context), error) {
	db, done, err := newEmptyDB(ctx)
	if err != nil {
		return nil, nil, err
	}

	sqlBytes, err := ioutil.ReadFile(trillianSQL)
	if err != nil {
		return nil, nil, err
	}

	for _, stmt := range strings.Split(sanitize(string(sqlBytes)), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, nil, fmt.Errorf("error running statement %q: %v", stmt, err)
		}
	}
	return db, done, nil
}

func sanitize(script string) string {
	buf := &bytes.Buffer{}
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || strings.Index(line, "--") == 0 {
			continue // skip empty lines and comments
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return buf.String()
}

// SkipIfNoMySQL is a test helper that skips tests that require a local MySQL.
func SkipIfNoMySQL(t *testing.T) {
	t.Helper()
	if !MySQLAvailable() {
		t.Skip("Skipping test as MySQL not available")
	}
	t.Logf("Test MySQL available at %q", mysqlURI())
}

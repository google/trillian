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

// Package testdb creates new databases for tests.
package testdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/google/trillian/testonly"

	_ "github.com/go-sql-driver/mysql" // mysql driver
)

var (
	trillianSQL = testonly.RelativeToPackage("../mysql/storage.sql")
	dataSource  = "root@tcp(127.0.0.1)/"
)

// MySQLAvailable indicates whether a default MySQL database is available.
func MySQLAvailable() bool {
	db, err := sql.Open("mysql", dataSource)
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

// newEmptyDB creates a new, empty database.
func newEmptyDB(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("mysql", dataSource)
	if err != nil {
		return nil, err
	}

	// Create a randomly-named database and then connect using the new name.
	name := fmt.Sprintf("trl_%v", time.Now().UnixNano())

	stmt := fmt.Sprintf("CREATE DATABASE %v", name)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
	}

	db.Close()
	db, err = sql.Open("mysql", dataSource+name)
	if err != nil {
		return nil, err
	}

	return db, db.Ping()
}

// NewTrillianDB creates an empty database with the Trillian schema. The database name is randomly
// generated.
// NewTrillianDB is equivalent to Default().NewTrillianDB(ctx).
func NewTrillianDB(ctx context.Context) (*sql.DB, error) {
	db, err := newEmptyDB(ctx)
	if err != nil {
		return nil, err
	}

	sqlBytes, err := ioutil.ReadFile(trillianSQL)
	if err != nil {
		return nil, err
	}

	for _, stmt := range strings.Split(sanitize(string(sqlBytes)), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
		}
	}
	return db, nil
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
}

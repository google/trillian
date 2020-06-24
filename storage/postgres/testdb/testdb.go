// Copyright 2018 Google LLC. All Rights Reserved.
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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/testonly"
)

var (
	trillianSQL = testonly.RelativeToPackage("../schema/storage.sql")
	pgOpts      = flag.String("pg_opts", "sslmode=disable", "Database options to be included when connecting to the db")
	dbName      = flag.String("db_name", "test", "The database name to be used when checking for pg connectivity")
)

// PGAvailable indicates whether a default PG database is available.
func PGAvailable() bool {
	db, err := sql.Open("postgres", getConnStr(*dbName))
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

// TestSQL executes a simple query in the configured database.  Only used as a placeholder
// for testing queries and how go returns results
func TestSQL(ctx context.Context) string {
	db, err := sql.Open("postgres", getConnStr(*dbName))
	if err != nil {
		fmt.Println("Error: ", err)
		return "error"
	}
	defer db.Close()
	result, err := db.QueryContext(ctx, "select 1=1")
	if err != nil {
		fmt.Println("Error: ", err)
		return "error"
	}
	var resultData bool
	result.Scan(&resultData)
	if resultData {
		fmt.Println("Result: ", resultData)
	}
	return "done"
}

// newEmptyDB creates a new, empty database.
// The returned clean up function should be called once the caller no longer
// needs the test DB.
func newEmptyDB(ctx context.Context) (*sql.DB, func(context.Context), error) {
	db, err := sql.Open("postgres", getConnStr(*dbName))
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
	db, err = sql.Open("postgres", getConnStr(name))
	if err != nil {
		return nil, nil, err
	}

	done := func(ctx context.Context) {
		defer db.Close()
		if _, err := db.ExecContext(ctx, "DROP DATABASE %v", name); err != nil {
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

	for _, stmt := range strings.Split(sanitize(string(sqlBytes)), ";--end") {
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

// sanitize tries to remove empty lines and comments from a sql script
// to prevent them from being executed.
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

func getConnStr(name string) string {
	return fmt.Sprintf("database=%s %s", name, *pgOpts)
}

// NewTrillianDBOrDie attempts to return a connection to a new postgres
// test database and fails if unable to do so.
func NewTrillianDBOrDie(ctx context.Context) (*sql.DB, func(context.Context)) {
	db, done, err := NewTrillianDB(ctx)
	if err != nil {
		panic(err)
	}
	return db, done
}

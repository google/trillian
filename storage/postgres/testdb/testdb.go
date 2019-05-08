// Copyright 2018 Google Inc. All Rights Reserved.
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
// To use: update the pg_opts in file or call with flags but be sure to include the host, user and updated port.  Also, in the get_conn below, be sure to
// update the password.  I didn't update that workflow because I'm not sure of all the places this test is used.
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

	"github.com/google/trillian/testonly"

	_ "github.com/lib/pq" // pg driver
)

var (
	trillianSQL = testonly.RelativeToPackage("../storage.sql")
	pgOpts      = flag.String("pg_opts", "host= user= port=5432", "Database options to be included when connecting to the db")
	dbName      = flag.String("db_name", "", "The database name to be used when checking for pg connectivity")
)

// PGAvailable indicates whether a default PG database is available.
func PGAvailable() bool {
	fmt.Println(getConnStr(*dbName))
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

//This just executes a simple query in the configured database.  Only used as a placeholder
// for testing queries and how go returns results
func TestSQL(ctx context.Context) string {
	db, err := sql.Open("postgres", getConnStr(*dbName))
	defer db.Close()
	if err != nil { 
		fmt.Println("Error: ",err)
                return "error"
        }
	result, err := db.QueryContext(ctx,"select 1=1")
	if err != nil {
		fmt.Println("Error: ",err)
		return "error"
	}
	var resultData bool
	result.Scan(&resultData)
	if resultData == true { 
		fmt.Println("Result: ",resultData)
	}
	return "done"
}


// newEmptyDB creates a new, empty database.
func newEmptyDB(ctx context.Context) (*sql.DB, error) {
	fmt.Println("new Db")
	db, err := sql.Open("postgres", getConnStr(*dbName))
	if err != nil {
		return nil, err
	}

	// Create a randomly-named database and then connect using the new name.
	name := fmt.Sprintf("trl_%v", time.Now().UnixNano())
	fmt.Println("Name",name)
	stmt := fmt.Sprintf("CREATE DATABASE %v", name)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
	}
	fmt.Println("past statement")
	db.Close()
	db, err = sql.Open("postgres", getConnStr(name))
	if err != nil {
		return nil, err
	}

	return db, db.Ping()
}

// NewTrillianDB creates an empty database with the Trillian schema. The database name is randomly
// generated.
// NewTrillianDB is equivalent to Default().NewTrillianDB(ctx).
func NewTrillianDB(ctx context.Context) (*sql.DB, error) {
	fmt.Println("Fail on fake context")
	db, err := newEmptyDB(ctx)
	if err != nil {
		return nil, err
	}
	
	fmt.Println(trillianSQL)
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
	return fmt.Sprintf("database=%s %s password=", name, *pgOpts)
}

// NewTrillianDBOrDie attempts to return a connection to a new postgres
// test database and fails if unable to do so.
func NewTrillianDBOrDie(ctx context.Context) *sql.DB {
	db, err := NewTrillianDB(ctx)
	if err != nil {
		panic(err)
	}
	return db
}

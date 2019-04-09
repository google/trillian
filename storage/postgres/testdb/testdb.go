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
	"encoding/json"

	"github.com/google/trillian/testonly"

	_ "github.com/lib/pq" // pg driver
)

var (
	trillianSQL = testonly.RelativeToPackage("../storage.sql")
	pgOpts      = flag.String("pg_opts", "sslmode=disable", "Database options to be included when connecting to the db")
	dbName      = flag.String("db_name", "test2", "The database name to be used when checking for pg connectivity")
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

 
func Test3(ctx context.Context) {
	fmt.Println("Start sql updtae")
	db, err := sql.Open("postgres", getConnStr(*dbName))

        if err != nil {
                log.Printf("sql.Open(): %v", err)
                //return false
        }
        defer db.Close()
	//var timestamp, treeSize, treeRevision int64
        //var rootHash, rootSignatureBytes []byte

//result, {"TreeSize":956308,"RootHash":"VzP81LYhQWbytZsfltjTQtpKxU14Ida69lmW7kdjpjE=","TimestampNanos":1554760730674199319,"Revision":219399,"Metadata":""} <nil>

	type DataTwo struct {
		TreeSize int64
		RootHash []byte
		TimestampNanos int64
		Revision int64
		MetaData []byte
	}
	data:= db.QueryRowContext(ctx,"select current_tree_data from trees limit 1")
	var jsonObj []byte
	data.Scan(&jsonObj)
	var d2 DataTwo
	json.Unmarshal(jsonObj,&d2)
	fmt.Println("result,", string(jsonObj),d2)
}

func Test2(ctx context.Context) {
	fmt.Println("Start sql updtae")
	db, err := sql.Open("postgres", getConnStr(*dbName))

        if err != nil {
                log.Printf("sql.Open(): %v", err)
                //return false
        }
        defer db.Close()

       type Message struct {
                Name string
                Body string
                Time int64
        }
        m := Message{"Alice", "Hello", 1294706395881547000}
	data,_ := json.Marshal(m)
	result, err := db.ExecContext(ctx,"update trees set current_tree_data = $1 where 1 = 1",data)
	fmt.Println("Result", result,err)
}

func TestSQL(ctx context.Context) string {
	fmt.Println("Starting test")
	db, err := sql.Open("postgres", getConnStr(*dbName))
	defer db.Close()
	if err != nil { 
		fmt.Println("Error: ",err)
                return "error"
        }
	result, err := db.QueryContext(ctx,"select ignore_duplicates(1531960917093437771,'f435e786ebc6694590fbd1790ca734299b5908c3b0044175d65c800d95def72d'::bytea,'ff'::bytea,'ff'::bytea,10)")
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
	return fmt.Sprintf("database=%s %s password=!@34QWerASdf", name, *pgOpts)
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

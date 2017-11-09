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
//
// Created databases may use either sqlite (default) or mysql as the database driver.
// The default driver can be overridden via the TRILLIAN_SQL_DRIVER environment variable.
package testdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/trillian/testonly"

	_ "github.com/go-sql-driver/mysql" // mysql driver
	_ "github.com/mattn/go-sqlite3"    // sqlite driver
)

const (
	mysqlDriver  = "mysql"
	sqliteDriver = "sqlite3"
)

var (
	trillianSQL = testonly.RelativeToPackage("../mysql/storage.sql")

	// enumRegex is used to replace ENUM columns with VARCHAR for sqlite.
	enumRegex *regexp.Regexp
)

func init() {
	var err error
	enumRegex, err = regexp.Compile(`ENUM\(.+\)`)
	if err != nil {
		panic(fmt.Sprintf("Failed to compile \"ENUM()\" regex: %v", err))
	}
}

// Default returns the default database provider.
func Default() *Provider {
	switch driver := os.Getenv("TRILLIAN_SQL_DRIVER"); driver {
	case mysqlDriver:
		return MySQL()
	case sqliteDriver, "":
		return SQLite()
	default:
		panic(fmt.Sprintf("Unsupported SQL driver for tests: %q", driver))
	}
}

// MySQL returns a MySQL database provider.
// The database must be running locally and have a root user without password.
func MySQL() *Provider {
	return &Provider{
		Driver:           mysqlDriver,
		DataSourceName:   "root@/",
		CreateDataSource: true,
	}
}

// SQLite returns a SQLite database provider.
func SQLite() *Provider {
	return &Provider{
		Driver: sqliteDriver,
		// TODO(codingllama): Move on to ":memory:" once we fix all the hangups
		// TODO(codingllama): Clean temporary files.
		DataSourceName:   fmt.Sprintf("file:%v/trl-%v", os.TempDir(), time.Now().UnixNano()),
		CreateDataSource: false,
	}
}

// New creates a new, empty database with a randomly generated name.
// New is equivalent to Default().New(ctx).
func New(ctx context.Context) (*sql.DB, error) {
	return Default().New(ctx)
}

// NewTrillianDB creates an empty database with the Trillian schema. The database name is randomly
// generated.
// NewTrillianDB is equivalent to Default().NewTrillianDB(ctx).
func NewTrillianDB(ctx context.Context) (*sql.DB, error) {
	return Default().NewTrillianDB(ctx)
}

// Provider is an object capable of creating new test databases.
type Provider struct {
	// Driver is the SQL driver name used for sql.Open (e.g.: "mysql" or "sqlite3").
	Driver string

	// DataSourceName is the data source name used for sql.Open (e.g., the database URL).
	DataSourceName string

	// CreateDataSource controls whether a new, random data source is created.
	// If set the true, New() firstly connects to DataSourceName, creates a randomly-generated
	// database (via "CREATE DATABASE") and then connects to the new database.
	// Useful to create random databases via a "main" data source name.
	CreateDataSource bool
}

// IsMySQL returns true if Provider uses the mysql driver.
func (p *Provider) IsMySQL() bool {
	return p.Driver == mysqlDriver
}

// New creates a new, empty database.
func (p *Provider) New(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open(p.Driver, p.DataSourceName)
	if err != nil {
		return nil, err
	}

	// If we're using MySQL (or something like it), first connect without an explicit DB, create a
	// randomly-named database and then connect using the new name.
	if p.CreateDataSource {
		name := fmt.Sprintf("trl_%v", time.Now().UnixNano())

		stmt := fmt.Sprintf("CREATE DATABASE %v", name)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
		}

		db.Close()
		// TODO(codingllama): The concatenation below may not work for drivers other than MySQL.
		db, err = sql.Open(p.Driver, p.DataSourceName+name)
		if err != nil {
			return nil, err
		}
	}

	return db, db.Ping()
}

// NewTrillianDB creates an empty database with the Trillian schema.
func (p *Provider) NewTrillianDB(ctx context.Context) (*sql.DB, error) {
	db, err := p.New(ctx)
	if err != nil {
		return nil, err
	}

	sqlBytes, err := ioutil.ReadFile(trillianSQL)
	if err != nil {
		return nil, err
	}

	for _, stmt := range strings.Split(p.sanitize(string(sqlBytes)), ";") {
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

func (p *Provider) sanitize(script string) string {
	buf := &bytes.Buffer{}
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || strings.Index(line, "--") == 0 {
			continue // skip empty lines and comments
		}
		if p.Driver == sqliteDriver {
			line = enumRegex.ReplaceAllString(line, "VARCHAR(50)")
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return buf.String()
}

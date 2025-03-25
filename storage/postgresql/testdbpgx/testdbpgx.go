// Copyright 2024 Trillian Authors. All Rights Reserved.
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

// Package testdbpgx creates new PostgreSQL databases for tests.
package testdbpgx

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/trillian/testonly"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

const (
	// PostgreSQLURIEnv is the name of the ENV variable checked for the test PostgreSQL
	// instance URI to use. The value must have a trailing slash.
	PostgreSQLURIEnv = "TEST_POSTGRESQL_URI"

	defaultTestPostgreSQLURI = "postgresql:///defaultdb?host=localhost&user=postgres&password=postgres"
)

type storageDriverInfo struct {
	schema  string
	uriFunc func(paths ...string) string
}

var (
	trillianPostgreSQLSchema = testonly.RelativeToPackage("../schema/storage.sql")
)

// DriverName is the name of a database driver.
type DriverName string

const (
	// DriverPostgreSQL is the identifier for the PostgreSQL storage driver.
	DriverPostgreSQL DriverName = "postgresql"
)

var driverMapping = map[DriverName]storageDriverInfo{
	DriverPostgreSQL: {
		schema:  trillianPostgreSQLSchema,
		uriFunc: postgresqlURI,
	},
}

// postgresqlURI returns the PostgreSQL connection URI to use for tests. It returns the
// value in the ENV variable defined by PostgreSQLURIEnv. If the value is empty,
// returns defaultTestPostgreSQLURI.
//
// We use an ENV variable, rather than a flag, for flexibility. Only a subset
// of the tests in this repo require a database and import this package. With a
// flag, it would be necessary to distinguish "go test" invocations that need a
// database, and those that don't. ENV allows to "blanket apply" this setting.
func postgresqlURI(dbRef ...string) string {
	var stringurl string
	if e := os.Getenv(PostgreSQLURIEnv); len(e) > 0 {
		stringurl = e
	} else {
		stringurl = defaultTestPostgreSQLURI
	}

	for _, ref := range dbRef {
		if strings.Contains(ref, "=") {
			separator := "&"
			if strings.HasSuffix(stringurl, "&") {
				separator = ""
			}
			stringurl = strings.Join([]string{stringurl, ref}, separator)
		} else {
			// No equals character, so use this string as the database name.
			if s1 := strings.SplitN(stringurl, "//", 2); len(s1) == 2 {
				if s2 := strings.SplitN(stringurl, "?", 2); len(s2) == 2 {
					stringurl = s1[0] + "///" + ref + "?" + s2[1]
				}
			}
		}
	}

	return stringurl
}

// PostgreSQLAvailable indicates whether the configured PostgreSQL database is available.
func PostgreSQLAvailable() bool {
	return dbAvailable(DriverPostgreSQL)
}

func dbAvailable(driver DriverName) bool {
	uri := driverMapping[driver].uriFunc()
	db, err := pgxpool.New(context.TODO(), uri)
	if err != nil {
		log.Printf("pgxpool.New(): %v", err)
		return false
	}
	defer db.Close()
	if err := db.Ping(context.TODO()); err != nil {
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
		return fmt.Errorf("could not set FD limit to %v. Must be less than the hard limit %v", uLimit, rLimit.Max)
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
func newEmptyDB(ctx context.Context, driver DriverName) (*pgxpool.Pool, func(context.Context), error) {
	if err := SetFDLimit(2048); err != nil {
		return nil, nil, err
	}

	inf, gotinf := driverMapping[driver]
	if !gotinf {
		return nil, nil, fmt.Errorf("unknown driver %q", driver)
	}

	db, err := pgxpool.New(ctx, inf.uriFunc())
	if err != nil {
		return nil, nil, err
	}

	// Create a randomly-named database and then connect using the new name.
	name := fmt.Sprintf("trl_%v", time.Now().UnixNano())

	stmt := fmt.Sprintf("CREATE DATABASE %v", name)
	if _, err := db.Exec(ctx, stmt); err != nil {
		return nil, nil, fmt.Errorf("error running statement %q: %v", stmt, err)
	}

	db.Close()
	uri := inf.uriFunc(name)
	db, err = pgxpool.New(ctx, uri)
	if err != nil {
		return nil, nil, err
	}

	done := func(ctx context.Context) {
		db.Close()
		if db, err = pgxpool.New(ctx, inf.uriFunc()); err != nil {
			klog.Warningf("Failed to reconnect: %v", err)
		}
		defer db.Close()
		if _, err := db.Exec(ctx, fmt.Sprintf("DROP DATABASE %v", name)); err != nil {
			klog.Warningf("Failed to drop test database %q: %v", name, err)
		}
	}

	return db, done, db.Ping(ctx)
}

// NewTrillianDB creates an empty database with the Trillian schema. The database name is randomly
// generated.
// NewTrillianDB is equivalent to Default().NewTrillianDB(ctx).
func NewTrillianDB(ctx context.Context, driver DriverName) (*pgxpool.Pool, func(context.Context), error) {
	db, done, err := newEmptyDB(ctx, driver)
	if err != nil {
		return nil, nil, err
	}

	schema := driverMapping[driver].schema

	sqlBytes, err := os.ReadFile(schema)
	if err != nil {
		return nil, nil, err
	}

	// Execute each statement in the schema file.  Each statement must end with a semicolon, and there must be a blank line before the next statement.
	for _, stmt := range strings.Split(sanitize(string(sqlBytes)), ";\n\n") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
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

// SkipIfNoPostgreSQL is a test helper that skips tests that require a local PostgreSQL.
func SkipIfNoPostgreSQL(t *testing.T) {
	t.Helper()
	if !PostgreSQLAvailable() {
		t.Skip("Skipping test as PostgreSQL not available")
	}
	t.Logf("Test PostgreSQL available at %q", postgresqlURI())
}

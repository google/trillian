// Copyright 2022 <TBD>
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

package crdb

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"sync"
	"testing"

	"github.com/cockroachdb/cockroach-go/v2/testserver"
	"k8s.io/klog/v2"

	"github.com/google/trillian/storage/testdb"
)

// testDBs holds a set of test databases, one per test.
var testDBs sync.Map

type testDBHandle struct {
	db   *sql.DB
	done func(context.Context)
}

func (db *testDBHandle) GetDB() *sql.DB {
	return db.db
}

func TestMain(m *testing.M) {
	flag.Parse()

	ts, err := testserver.NewTestServer()
	if err != nil {
		klog.Errorf("Failed to start test server: %v", err)
		os.Exit(1)
	}
	defer ts.Stop()

	// reset the test server URL path. By default cockroach sets it
	// to point to a default database, we don't want that.
	dburl := ts.PGURL()
	dburl.Path = "/"

	// Set the environment variable for the test server
	os.Setenv(testdb.CockroachDBURIEnv, dburl.String())

	if !testdb.CockroachDBAvailable() {
		klog.Errorf("CockroachDB not available, skipping all CockroachDB storage tests")
		return
	}

	status := m.Run()

	// Clean up databases
	testDBs.Range(func(key, value interface{}) bool {
		testName := key.(string)
		klog.Infof("Cleaning up database for test %s", testName)

		db := value.(*testDBHandle)

		// TODO(jaosorior): Set a timeout instead of using Background
		db.done(context.Background())

		return true
	})

	os.Exit(status)
}

// This is used to identify a database from the map
func getDBID(t *testing.T) string {
	t.Helper()
	return t.Name()
}

func openTestDBOrDie(t *testing.T) *testDBHandle {
	t.Helper()

	// TODO(jaosorior): Check for Cockroach
	db, done, err := testdb.NewTrillianDB(context.TODO(), testdb.DriverCockroachDB)
	if err != nil {
		panic(err)
	}

	handle := &testDBHandle{
		db:   db,
		done: done,
	}

	testDBs.Store(getDBID(t), handle)

	return handle
}

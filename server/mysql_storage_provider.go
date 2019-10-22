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

package server

import (
	"database/sql"
	"flag"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"

	// Load MySQL driver
	_ "github.com/go-sql-driver/mysql"
)

var (
	mySQLURI = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	maxConns = flag.Int("mysql_max_conns", 0, "Maximum connections to the database")
	maxIdle  = flag.Int("mysql_max_idle_conns", -1, "Maximum idle database connections in the connection pool")

	mysqlMu              sync.Mutex
	mysqlErr             error
	mysqlDB              *sql.DB
	mysqlStorageInstance *mysqlProvider
)

func init() {
	if err := RegisterStorageProvider("mysql", newMySQLStorageProvider); err != nil {
		glog.Fatalf("Failed to register storage provider mysql: %v", err)
	}
}

type mysqlProvider struct {
	db *sql.DB
	mf monitoring.MetricFactory
}

func newMySQLStorageProvider(mf monitoring.MetricFactory) (StorageProvider, error) {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	if mysqlStorageInstance == nil {
		db, err := getMySQLDatabaseLocked()
		if err != nil {
			return nil, err
		}
		mysqlStorageInstance = &mysqlProvider{
			db: db,
			mf: mf,
		}
	}
	return mysqlStorageInstance, nil
}

// getMySQLDatabase returns an instance of MySQL database, or creates one.
func getMySQLDatabase() (*sql.DB, error) {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	return getMySQLDatabaseLocked()
}

// getMySQLDatabaseLocked returns an instance of MySQL database, or creates
// one. Requires mysqlMu to be locked.
func getMySQLDatabaseLocked() (*sql.DB, error) {
	if mysqlDB != nil || mysqlErr != nil {
		return mysqlDB, mysqlErr
	}
	db, err := mysql.OpenDB(*mySQLURI)
	if err != nil {
		mysqlErr = err
		return nil, err
	}
	if *maxConns > 0 {
		db.SetMaxOpenConns(*maxConns)
	}
	if *maxIdle >= 0 {
		db.SetMaxIdleConns(*maxIdle)
	}
	mysqlDB, mysqlErr = db, nil
	return db, nil
}

func (s *mysqlProvider) LogStorage() storage.LogStorage {
	return mysql.NewLogStorage(s.db, s.mf)
}

func (s *mysqlProvider) MapStorage() storage.MapStorage {
	return mysql.NewMapStorage(s.db)
}

func (s *mysqlProvider) AdminStorage() storage.AdminStorage {
	return mysql.NewAdminStorage(s.db)
}

func (s *mysqlProvider) Close() error {
	return s.db.Close()
}

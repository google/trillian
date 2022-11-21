// Copyright 2018 Trillian Authors
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
	"database/sql"
	"flag"
	"sync"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"k8s.io/klog/v2"

	_ "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgx" // crdb retries and postgres interface
	_ "github.com/lib/pq"                                   // Register the Postgres driver.
)

const (
	// StorageProviderName is the name of the storage provider.
	StorageProviderName = "crdb"
)

var (
	crdbURI  = flag.String("crdb_uri", "postgresql://root@localhost:26257?sslmode=disable", "Connection URI for CockroachDB database")
	maxConns = flag.Int("crdb_max_conns", 0, "Maximum connections to the database")
	maxIdle  = flag.Int("crdb_max_idle_conns", -1, "Maximum idle database connections in the connection pool")

	crdbErr             error
	crdbHandle          *sql.DB
	crdbStorageInstance *crdbProvider
	dbConnMu            sync.Mutex
)

// GetDatabase returns the database handle for the provider.
func GetDatabase() (*sql.DB, error) {
	dbConnMu.Lock()
	defer dbConnMu.Unlock()
	return getCRDBDatabaseLocked()
}

func init() {
	if err := storage.RegisterProvider(StorageProviderName, newCRDBStorageProvider); err != nil {
		klog.Fatalf("Failed to register storage provider crdb: %v", err)
	}
}

type crdbProvider struct {
	db *sql.DB
	mf monitoring.MetricFactory
}

func newCRDBStorageProvider(mf monitoring.MetricFactory) (storage.Provider, error) {
	dbConnMu.Lock()
	defer dbConnMu.Unlock()
	if crdbStorageInstance == nil {
		db, err := getCRDBDatabaseLocked()
		if err != nil {
			return nil, err
		}
		crdbStorageInstance = &crdbProvider{
			db: db,
			mf: mf,
		}
	}

	return crdbStorageInstance, nil
}

// Lazy initializes the database connection handle and returns the instance.
// Requires lock to be held.
func getCRDBDatabaseLocked() (*sql.DB, error) {
	if crdbHandle != nil || crdbErr != nil {
		return crdbHandle, crdbErr
	}
	db, err := OpenDB(*crdbURI)
	if err != nil {
		crdbErr = err
		return nil, err
	}
	if *maxConns > 0 {
		db.SetMaxOpenConns(*maxConns)
	}
	if *maxIdle >= 0 {
		db.SetMaxIdleConns(*maxIdle)
	}
	crdbHandle, crdbErr = db, nil
	return db, nil
}

func (p *crdbProvider) Close() error {
	return p.db.Close()
}

func (p *crdbProvider) LogStorage() storage.LogStorage {
	return NewLogStorage(p.db, p.mf)
}

func (p *crdbProvider) AdminStorage() storage.AdminStorage {
	return NewSQLAdminStorage(p.db)
}

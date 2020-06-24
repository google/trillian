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

package postgres

import (
	"database/sql"
	"flag"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"

	// Load PG driver
	_ "github.com/lib/pq"
)

var (
	pgConnStr         = flag.String("pg_conn_str", "user=postgres dbname=test port=5432 sslmode=disable", "Connection string for Postgres database")
	pgOnce            sync.Once
	pgOnceErr         error
	pgStorageInstance *pgProvider
)

func init() {
	if err := storage.RegisterProvider("postgres", newPGProvider); err != nil {
		glog.Fatalf("Failed to register storage provider postgres: %v", err)
	}
}

type pgProvider struct {
	db *sql.DB
	mf monitoring.MetricFactory
}

func newPGProvider(mf monitoring.MetricFactory) (storage.Provider, error) {
	pgOnce.Do(func() {
		var db *sql.DB
		db, pgOnceErr = OpenDB(*pgConnStr)
		if pgOnceErr != nil {
			return
		}

		pgStorageInstance = &pgProvider{
			db: db,
			mf: mf,
		}
	})
	if pgOnceErr != nil {
		return nil, pgOnceErr
	}
	return pgStorageInstance, nil
}

func (s *pgProvider) LogStorage() storage.LogStorage {

	glog.Warningf("Support for the PostgreSQL log is experimental.  Please use at your own risk!!!")
	return NewLogStorage(s.db, s.mf)
}

func (s *pgProvider) MapStorage() storage.MapStorage {
	panic("Not Implemented")
}

func (s *pgProvider) AdminStorage() storage.AdminStorage {
	return NewAdminStorage(s.db)
}

func (s *pgProvider) Close() error {
	return s.db.Close()
}

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
	"github.com/google/trillian/storage/postgres"

	// Load MySQL driver
	_ "github.com/lib/pq"
)

var (
	pgConnStr         = flag.String("pg_uri", "user=postgres dbname=test port=5432 sslmode=disable", "Connection string for Postgres database")
	pgOnce            sync.Once
	pgStorageInstance *pgProvider
)

func init() {
	if err := RegisterStorageProvider("postgres", newPGProvider); err != nil {
		glog.Fatalf("Failed to register storage provider postgres: %v", err)
	}
}

type pgProvider struct {
	db *sql.DB
	mf monitoring.MetricFactory
}

func newPGProvider(mf monitoring.MetricFactory) (StorageProvider, error) {
	var err error

	pgOnce.Do(func() {
		var db *sql.DB
		db, err = postgres.OpenDB(*pgConnStr)
		if err != nil {
			return
		}

		pgStorageInstance = &pgProvider{
			db: db,
			mf: mf,
		}
	})
	if err != nil {
		return nil, err
	}
	return pgStorageInstance, nil
}

func (s *pgProvider) LogStorage() storage.LogStorage {
	panic("Not Implemented")
}

func (s *pgProvider) MapStorage() storage.MapStorage {
	panic("Not Implemented")
}

func (s *pgProvider) AdminStorage() storage.AdminStorage {
	return postgres.NewAdminStorage(s.db)
}

func (s *pgProvider) Close() error {
	return s.db.Close()
}

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

package postgresql

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"os"
	"sync"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"k8s.io/klog/v2"

	// Load PostgreSQL driver
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	postgreSQLURI        = flag.String("postgresql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for PostgreSQL database")
	maxConns        = flag.Int("postgresql_max_conns", 0, "Maximum connections to the database")
	maxIdle         = flag.Int("postgresql_max_idle_conns", -1, "Maximum idle database connections in the connection pool")
	postgreSQLTLSCA      = flag.String("postgresql_tls_ca", "", "Path to the CA certificate file for PostgreSQL TLS connection ")
	postgreSQLServerName = flag.String("postgresql_server_name", "", "Name of the PostgreSQL server to be used as the Server Name in the TLS configuration")

	postgresqlMu              sync.Mutex
	postgresqlErr             error
	postgresqlDB              *pgxpool.Pool
	postgresqlStorageInstance *postgresqlProvider
)

// GetDatabase returns an instance of PostgreSQL database, or creates one.
//
// TODO(pavelkalinnikov): Make the dependency of PostgreSQL quota provider from
// PostgreSQL storage provider explicit.
func GetDatabase() (*pgxpool.Pool, error) {
	postgresqlMu.Lock()
	defer postgresqlMu.Unlock()
	return getPostgreSQLDatabaseLocked()
}

func init() {
	if err := storage.RegisterProvider("postgresql", newPostgreSQLStorageProvider); err != nil {
		klog.Fatalf("Failed to register storage provider postgresql: %v", err)
	}
}

type postgresqlProvider struct {
	db *pgxpool.Pool
	mf monitoring.MetricFactory
}

func newPostgreSQLStorageProvider(mf monitoring.MetricFactory) (storage.Provider, error) {
	postgresqlMu.Lock()
	defer postgresqlMu.Unlock()
	if postgresqlStorageInstance == nil {
		db, err := getPostgreSQLDatabaseLocked()
		if err != nil {
			return nil, err
		}
		postgresqlStorageInstance = &postgresqlProvider{
			db: db,
			mf: mf,
		}
	}
	return postgresqlStorageInstance, nil
}

// getPostgreSQLDatabaseLocked returns an instance of PostgreSQL database, or creates
// one. Requires postgresqlMu to be locked.
func getPostgreSQLDatabaseLocked() (*pgxpool.Pool, error) {
	if postgresqlDB != nil || postgresqlErr != nil {
		return postgresqlDB, postgresqlErr
	}
	dsn := *postgreSQLURI
	if *postgreSQLTLSCA != "" {
		if err := registerPostgreSQLTLSConfig(); err != nil {
			return nil, err
		}
		dsn += "?tls=custom"
	}
	db, err := OpenDB(dsn)
	if err != nil {
		postgresqlErr = err
		return nil, err
	}
	if *maxConns > 0 {
		db.SetMaxOpenConns(*maxConns)
	}
	if *maxIdle >= 0 {
		db.SetMaxIdleConns(*maxIdle)
	}
	postgresqlDB, postgresqlErr = db, nil
	return db, nil
}

func (s *postgresqlProvider) LogStorage() storage.LogStorage {
	return NewLogStorage(s.db, s.mf)
}

func (s *postgresqlProvider) AdminStorage() storage.AdminStorage {
	return NewAdminStorage(s.db)
}

func (s *postgresqlProvider) Close() error {
	s.db.Close()
	return nil
}

// registerPostgreSQLTLSConfig registers a custom TLS config for PostgreSQL using a provided CA certificate and optional server name.
// Returns an error if the CA certificate can't be read or added to the root cert pool, or when the registration of the TLS config fails.
func registerPostgreSQLTLSConfig() error {
	if *postgreSQLTLSCA == "" {
		return nil
	}
	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile(*postgreSQLTLSCA)
	if err != nil {
		return err
	}
	if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
		return errors.New("failed to append PEM")
	}
	tlsConfig := &tls.Config{
		RootCAs: rootCertPool,
	}
	if *postgreSQLServerName != "" {
		tlsConfig.ServerName = *postgreSQLServerName
	}
	return postgresql.RegisterTLSConfig("custom", tlsConfig)
}

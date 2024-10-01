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

package mysql

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"flag"
	"os"
	"sync"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"k8s.io/klog/v2"

	// Load MySQL driver
	"github.com/go-sql-driver/mysql"
)

var (
	mySQLURI        = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	maxConns        = flag.Int("mysql_max_conns", 0, "Maximum connections to the database")
	maxIdle         = flag.Int("mysql_max_idle_conns", -1, "Maximum idle database connections in the connection pool")
	mySQLTLSCA      = flag.String("mysql_tls_ca", "", "Path to the CA certificate file for MySQL TLS connection ")
	mySQLServerName = flag.String("mysql_server_name", "", "Name of the MySQL server to be used as the Server Name in the TLS configuration")

	mysqlMu              sync.Mutex
	mysqlErr             error
	mysqlDB              *sql.DB
	mysqlStorageInstance *mysqlProvider
)

// GetDatabase returns an instance of MySQL database, or creates one.
//
// TODO(pavelkalinnikov): Make the dependency of MySQL quota provider from
// MySQL storage provider explicit.
func GetDatabase() (*sql.DB, error) {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	return getMySQLDatabaseLocked()
}

func init() {
	if err := storage.RegisterProvider("mysql", newMySQLStorageProvider); err != nil {
		klog.Fatalf("Failed to register storage provider mysql: %v", err)
	}
}

type mysqlProvider struct {
	db *sql.DB
	mf monitoring.MetricFactory
}

func newMySQLStorageProvider(mf monitoring.MetricFactory) (storage.Provider, error) {
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

// getMySQLDatabaseLocked returns an instance of MySQL database, or creates
// one. Requires mysqlMu to be locked.
func getMySQLDatabaseLocked() (*sql.DB, error) {
	if mysqlDB != nil || mysqlErr != nil {
		return mysqlDB, mysqlErr
	}
	dsn := *mySQLURI
	if *mySQLTLSCA != "" {
		if err := registerMySQLTLSConfig(); err != nil {
			return nil, err
		}
		dsn += "?tls=custom"
	}
	db, err := OpenDB(dsn)
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
	return NewLogStorage(s.db, s.mf)
}

func (s *mysqlProvider) AdminStorage() storage.AdminStorage {
	return NewAdminStorage(s.db)
}

func (s *mysqlProvider) Close() error {
	return s.db.Close()
}

// registerMySQLTLSConfig registers a custom TLS config for MySQL using a provided CA certificate and optional server name.
// Returns an error if the CA certificate can't be read or added to the root cert pool, or when the registration of the TLS config fails.
func registerMySQLTLSConfig() error {
	if *mySQLTLSCA == "" {
		return nil
	}
	rootCertPool := x509.NewCertPool()
	pem, err := os.ReadFile(*mySQLTLSCA)
	if err != nil {
		return err
	}
	if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
		return errors.New("failed to append PEM")
	}
	tlsConfig := &tls.Config{
		RootCAs: rootCertPool,
	}
	if *mySQLServerName != "" {
		tlsConfig.ServerName = *mySQLServerName
	}
	return mysql.RegisterTLSConfig("custom", tlsConfig)
}

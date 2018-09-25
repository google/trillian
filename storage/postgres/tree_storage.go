package postgres

import (
	"database/sql"

	"github.com/golang/glog"
)

// OpenDB opens a database connection for all MySQL-based storage implementations.
func OpenDB(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		glog.Warningf("Could not open Postgres database, check config: %s", err)
		return nil, err
	}

	return db, nil
}

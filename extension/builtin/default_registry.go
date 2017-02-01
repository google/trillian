package builtin

import (
	"database/sql"
	"flag"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

// MySQLURIFlag is the mysql db connection string.
var MySQLURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")

// Default implementation of extension.Registry.
type defaultRegistry struct {
	db *sql.DB
}

func (r *defaultRegistry) GetLogStorage() (storage.LogStorage, error) {
	return mysql.NewLogStorage(r.db)
}

func (r *defaultRegistry) GetMapStorage() (storage.MapStorage, error) {
	return mysql.NewMapStorage(r.db)
}

// NewDefaultExtensionRegistry returns the default extension.Registry implementation, which is
// backed by a MySQL database and configured via flags.
// The returned registry is wraped in a cached registry.
func NewDefaultExtensionRegistry() (extension.Registry, error) {
	db, err := mysql.OpenDB(*MySQLURIFlag)
	if err != nil {
		return nil, err
	}
	return &defaultRegistry{
		db: db,
	}, nil
}

package builtin

import (
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
type defaultRegistry struct{}

func (r defaultRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	return mysql.NewLogStorage(treeID, *MySQLURIFlag)
}

func (r defaultRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	return mysql.NewMapStorage(treeID, *MySQLURIFlag)
}

// NewDefaultExtensionRegistry returns the default extension.Registry implementation, which is
// backed by a MySQL database and configured via flags.
// The returned registry is wraped in a cached registry.
func NewDefaultExtensionRegistry() (extension.Registry, error) {
	return extension.NewCachedRegistry(defaultRegistry{}), nil
}

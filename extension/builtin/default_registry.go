package builtin

import (
	"flag"

	_ "github.com/go-sql-driver/mysql"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var mysqlURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "uri to use with mysql storage")

// Default implementation of ExtensionRegistry.
type defaultRegistry struct{}

func (r defaultRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	return mysql.NewLogStorage(treeID, *mysqlURIFlag)
}

func (r defaultRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	return mysql.NewMapStorage(treeID, *mysqlURIFlag)
}

// NewDefaultExtensionRegistry returns the default ExtensionRegistry implementation, which is backed
// up by a MySQL database and configured via flags.
// The returned registry is wraped in a cached registry.
func NewDefaultExtensionRegistry() (extension.ExtensionRegistry, error) {
	return extension.NewCachedExtensionRegistry(defaultRegistry{}), nil
}

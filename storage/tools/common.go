package tools

import (
	"flag"
	"fmt"

	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var treeIDFlag = flag.Int64("treeid", 3, "The tree id to use")
var storageTypeFlag = flag.String("storage_type", "mysql", "Which type of storage to use")
var mysqlURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")
var serverPortFlag = flag.Int("port", 8090, "Port to serve log requests on")

// GetLogIDFromFlagsOrDie returns the Trillian LogID from the current flags configuration.
func GetLogIDFromFlagsOrDie() int64 {
	return *treeIDFlag
}

// GetLogStorageProviderFromFlags returns a storage provider configured from our
// flag settings.
// TODO: This needs to be tidied up
func GetLogStorageProviderFromFlags() func(x int64) (storage.LogStorage, error) {
	return func(x int64) (storage.LogStorage, error) {
		storageProvider, err := GetStorageFromFlags(x)
		return storageProvider, err
	}
}

// GetStorageFromFlags returns a configured storage instance, this can fail with an error
func GetStorageFromFlags(treeID int64) (storage.LogStorage, error) {
	switch {
	case *storageTypeFlag == "mysql":
		store, err := mysql.NewLogStorage(treeID, *mysqlURIFlag)

		if err != nil {
			panic(err)
		}

		return store, err
	}

	return nil, fmt.Errorf("Unknown storage type: %s", *storageTypeFlag)
}

// GetStorageFromFlagsOrDie returns a configured storage instance, errors are fatal and if it
// returns the storage can be used.
func GetStorageFromFlagsOrDie(treeID int64) storage.LogStorage {
	logStorage, err := GetStorageFromFlags(treeID)

	if err != nil {
		panic(err)
	}

	return logStorage
}

// GetMapStorageFromFlags returns a configured MapStorage instance or an error.
func GetMapStorageFromFlags(treeID int64) (storage.MapStorage, error) {
	return mysql.NewMapStorage(treeID, *mysqlURIFlag)
}

// GetLogServerPort returns the port number to be used when serving log data
func GetLogServerPort() int {
	return *serverPortFlag
}

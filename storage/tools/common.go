package tools

import (
	"flag"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var logIdFlag = flag.String("logid", "logId", "The log id to use")
var treeIdFlag = flag.Int64("treeid", 3, "The tree id to use")
var storageTypeFlag = flag.String("storage_type", "mysql", "Which type of storage to use")
var mysqlUriFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")
var serverPortFlag = flag.Int("port", 8090, "Port to serve log requests on")

func GetLogIdFromFlagsOrDie() trillian.LogID {
	return trillian.LogID{[]byte(*logIdFlag), *treeIdFlag}
}

// GetLogStorageProviderFromFlags returns a storage provider configured from our
// flag settings.
// TODO: This needs to be tidied up
func GetLogStorageProviderFromFlags() server.LogStorageProviderFunc {
	return func(x int64) (storage.LogStorage, error) {
		// TODO: We need to sort out exactly what a log id is. We really only need a tree id
		// but having the log id as well could be a handy cross check as they can be guessed
		storage, err := GetStorageFromFlags(trillian.LogID{[]byte("This needs fixing"), x})
		return storage, err
	}
}

// GetStorageFromFlags returns a configured storage instance, this can fail with an error
func GetStorageFromFlags(treeId trillian.LogID) (storage.LogStorage, error) {
	switch {
	case *storageTypeFlag == "mysql":
		store, err := mysql.NewLogStorage(treeId, *mysqlUriFlag)

		if err != nil {
			panic(err)
		}

		return store, err
	}

	return nil, fmt.Errorf("Unknown storage type: %s", *storageTypeFlag)
}

// GetStorageFromFlagsOrDie returns a configured storage instance, errors are fatal and if it
// returns the storage can be used.
func GetStorageFromFlagsOrDie(treeId trillian.LogID) storage.LogStorage {
	storage, err := GetStorageFromFlags(treeId)

	if err != nil {
		panic(err)
	}

	return storage
}

// GetLogServerPort returns the port number to be used when serving log data
func GetLogServerPort() int {
	return *serverPortFlag
}

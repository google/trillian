package main

import (
	"flag"
	"fmt"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var logIdFlag = flag.String("logid", "logId", "The log id to use")
var treeIdFlag = flag.Int64("treeid", 3, "The tree id to use")
var storageTypeFlag = flag.String("storage_type", "mysql", "Which type of storage to use")
var mysqlUriFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")

func getLogIdFromFlagsOrDie() trillian.LogID {
	return trillian.LogID{[]byte(*logIdFlag), *treeIdFlag}
}

func getStorageFromFlagsOrDie(treeId trillian.LogID) storage.LogStorage {
	switch {
	case *storageTypeFlag == "mysql":
		store, err := mysql.NewLogStorage(treeId, *mysqlUriFlag)

		if err != nil {
			panic(err)
		}

		return store
	}

	panic(fmt.Sprintf("Unknown storage type: %s", *storageTypeFlag))
}

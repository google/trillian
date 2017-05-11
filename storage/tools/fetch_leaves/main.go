// Copyright 2016 Google Inc. All Rights Reserved.
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

package main

import (
	"context"
	"encoding/hex"
	"flag"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	log "github.com/golang/glog"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/db"
)

var (
	dbDriver           = flag.String("db_driver", "mysql", "Database driver name")
	dbURI              = flag.String("db_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for database")
	treeIDFlag         = flag.Int64("treeid", 3, "The tree id to use")
	fetchLeavesFlag    = flag.Int("fetch_leaves", 1, "Number of entries to fetch")
	startFetchFromFlag = flag.Int("start_fetch_at", 0, "The sequence number of the first leaf to fetch")
	leafHashHex        = flag.String("leaf_hash", "", "The hash of a leaf to fetch")
)

func validateFetchFlagsOrDie() {
	if *fetchLeavesFlag <= 0 {
		panic("Invalid value for num_insertions")
	}

	if *startFetchFromFlag < 0 {
		panic("Invalid value for start_fetch_at")
	}
}

// Fetches some leaves from storage to exercise the API. Batching can be added later if need be.
func main() {
	flag.Parse()
	validateFetchFlagsOrDie()

	wrap, err := db.OpenDB(*dbDriver, *dbURI)
	if err != nil {
		log.Exitf("Failed to open database: %v", err)
	}
	defer wrap.DB().Close()

	storage := coresql.NewLogStorage(wrap)
	ctx := context.Background()
	tx, err := storage.SnapshotForTree(ctx, *treeIDFlag)
	if err != nil {
		panic(err)
	}

	leafCount, err := tx.GetSequencedLeafCount(ctx)
	if err != nil {
		panic(err)
	}
	log.Infof("Sequenced leaf count in storage is: %d", leafCount)

	if len(*leafHashHex) > 0 {
		hash, err := hex.DecodeString(*leafHashHex)
		if err != nil {
			panic(err)
		}

		fetchedLeaves, err := tx.GetLeavesByHash(ctx, [][]byte{hash}, false)
		if err != nil {
			panic(err)
		}

		for index, leaf := range fetchedLeaves {
			log.Infof("Result %d: %v", index, leaf)
		}
	} else {
		leaves := []int64{}

		for l := 0; l < *fetchLeavesFlag; l++ {
			// Leaf data based in the sequence number so we can check the hashes
			leafNumber := *startFetchFromFlag + l

			leaves = append(leaves, int64(leafNumber))
		}

		fetchedLeaves, err := tx.GetLeavesByIndex(ctx, leaves)
		if err != nil {
			panic(err)
		}

		for index, leaf := range fetchedLeaves {
			log.Infof("Result %d for leaf %d: %v", index, leaves[index], leaf)
		}
	}

	if err := tx.Commit(); err != nil {
		panic(err)
	}
}

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

	log "github.com/golang/glog"
	"github.com/google/trillian/extension/builtin"
)

var treeIDFlag = flag.Int64("treeid", 3, "The tree id to use")
var fetchLeavesFlag = flag.Int("fetch_leaves", 1, "Number of entries to fetch")
var startFetchFromFlag = flag.Int("start_fetch_at", 0, "The sequence number of the first leaf to fetch")
var leafHashHex = flag.String("leaf_hash", "", "The hash of a leaf to fetch")

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

	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		panic(err)
	}
	storage, err := registry.GetLogStorage()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	tx, err := storage.SnapshotForTree(ctx, *treeIDFlag)
	if err != nil {
		panic(err)
	}

	leafCount, err := tx.GetSequencedLeafCount()
	if err != nil {
		panic(err)
	}
	log.Infof("Sequenced leaf count in storage is: %d", leafCount)

	if len(*leafHashHex) > 0 {
		hash, err := hex.DecodeString(*leafHashHex)
		if err != nil {
			panic(err)
		}

		fetchedLeaves, err := tx.GetLeavesByHash([][]byte{hash}, false)
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

		fetchedLeaves, err := tx.GetLeavesByIndex(leaves)
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

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
	"crypto/sha256"
	"flag"
	"fmt"
	"time"

	log "github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension/builtin"
)

var treeIDFlag = flag.Int64("treeid", 3, "The tree id to use")
var numInsertionsFlag = flag.Int("num_insertions", 10, "Number of entries to insert in the tree")
var startInsertFromFlag = flag.Int("start_from", 0, "The sequence number of the first inserted item")
var queueBatchSizeFlag = flag.Int("queue_batch_size", 50, "Queue leaves batch size")

func validateFlagsOrDie() {
	if *numInsertionsFlag <= 0 {
		panic("Invalid value for num_insertions")
	}

	if *startInsertFromFlag < 0 {
		panic("Invalid value for start_from")
	}

	if *queueBatchSizeFlag <= 0 {
		panic("Invalid value for queue_batch_size")
	}
}

// Queues a number of leaves for a log from a given start point with predictable hashes.
// If anything fails it panics, leaving storage untouched
func main() {
	flag.Parse()
	validateFlagsOrDie()

	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		panic(err)
	}
	storage, err := registry.GetLogStorage()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	tx, err := storage.BeginForTree(ctx, *treeIDFlag)
	if err != nil {
		panic(err)
	}

	leaves := []*trillian.LogLeaf{}
	for l := 0; l < *numInsertionsFlag; l++ {
		// Leaf data based in the sequence number so we can check the hashes
		leafNumber := *startInsertFromFlag + l

		data := []byte(fmt.Sprintf("Leaf %d", leafNumber))
		hash := sha256.Sum256(data)

		log.Infof("Preparing leaf %d\n", leafNumber)

		leaf := &trillian.LogLeaf{
			MerkleLeafHash: []byte(hash[:]),
			LeafValue:      data,
			ExtraData:      nil,
			LeafIndex:      0}
		leaves = append(leaves, leaf)

		if len(leaves) >= *queueBatchSizeFlag {
			err = tx.QueueLeaves(leaves, time.Now())
			leaves = leaves[:0] // starting new batch

			if err != nil {
				panic(err)
			}
		}
	}

	// There might be some leaves left over that didn't get queued yet
	if len(leaves) > 0 {
		if err := tx.QueueLeaves(leaves, time.Now()); err != nil {
			panic(err)
		}
	}

	if err := tx.Commit(); err != nil {
		panic(err)
	}
}

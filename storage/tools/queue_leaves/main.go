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
	storage, err := registry.GetLogStorage(*treeIDFlag)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	tx, err := storage.Begin(ctx)

	if err != nil {
		panic(err)
	}

	leaves := []trillian.LogLeaf{}

	for l := 0; l < *numInsertionsFlag; l++ {
		// Leaf data based in the sequence number so we can check the hashes
		leafNumber := *startInsertFromFlag + l

		data := []byte(fmt.Sprintf("Leaf %d", leafNumber))
		hash := sha256.Sum256(data)

		log.Infof("Preparing leaf %d\n", leafNumber)

		leaf := trillian.LogLeaf{
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
		err = tx.QueueLeaves(leaves, time.Now())

		if err != nil {
			panic(err)
		}
	}

	err = tx.Commit()

	if err != nil {
		panic(err)
	}
}

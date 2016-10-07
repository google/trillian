package main

import (
	"crypto/sha256"
	"flag"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/storage/tools"
)

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

	treeID := tools.GetLogIDFromFlagsOrDie()
	storage := tools.GetStorageFromFlagsOrDie(treeID)

	tx, err := storage.Begin()

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
			Leaf: trillian.Leaf{
				LeafHash:  trillian.Hash(hash[:]),
				LeafValue: data,
				ExtraData: nil,
			},
			SequenceNumber: 0}
		leaves = append(leaves, leaf)

		if len(leaves) >= *queueBatchSizeFlag {
			err = tx.QueueLeaves(leaves)
			leaves = leaves[:0] // starting new batch

			if err != nil {
				panic(err)
			}
		}
	}

	// There might be some leaves left over that didn't get queued yet
	if len(leaves) > 0 {
		err = tx.QueueLeaves(leaves)

		if err != nil {
			panic(err)
		}
	}

	err = tx.Commit()

	if err != nil {
		panic(err)
	}
}

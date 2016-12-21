package main

import (
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
	storage, err := registry.GetLogStorage(*treeIDFlag)
	if err != nil {
		panic(err)
	}

	tx, err := storage.Begin()

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

	err = tx.Commit()

	if err != nil {
		panic(err)
	}
}

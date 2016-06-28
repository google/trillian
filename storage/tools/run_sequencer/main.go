package main

import (
	"flag"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/trillian"
	"github.com/google/trillian/log"
	"github.com/google/trillian/storage/tools"
	"github.com/google/trillian/util"
)

var batchLimitFlag = flag.Int("batch_limit", 50, "Max number of leaves to process")

// This just runs a one shot sequencing operation. Use queue_leaves to prepare work to
// and then run this.
func main() {
	flag.Parse()

	logID := tools.GetLogIdFromFlagsOrDie()
	storage := tools.GetStorageFromFlagsOrDie(logID)

	sequencer := log.NewSequencer(trillian.NewSHA256(), new(util.SystemTimeSource), storage)

	_, err := sequencer.SequenceBatch(*batchLimitFlag)

	if err != nil {
		panic(err)
	}
}

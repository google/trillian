package integration

import (
	"flag"
	"testing"
	"time"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

var treeIDFlag = flag.Int64("treeid", -1, "The tree id to use")
var serverFlag = flag.String("log_rpc_server", "localhost:8092", "Server address:port")
var queueLeavesFlag = flag.Bool("queue_leaves", true, "If true queues leaves, false just reads from the log")
var awaitSequencingFlag = flag.Bool("await_sequencing", true, "If true then waits until log size is at least num_leaves")
var checkLogEmptyFlag = flag.Bool("check_log_empty", true, "If true ensures log is empty before queuing anything")
var startLeafFlag = flag.Int64("start_leaf", 0, "The first leaf index to use")
var numLeavesFlag = flag.Int64("num_leaves", 1000, "The number of leaves to submit and read back")
var queueBatchSizeFlag = flag.Int("queue_batch_size", 50, "Batch size when queueing leaves")
var sequencerBatchSizeFlag = flag.Int("sequencing_batch_size", 100, "Batch size for server sequencer")
var readBatchSizeFlag = flag.Int64("read_batch_size", 50, "Batch size when getting leaves by index")
var waitForSequencingFlag = flag.Duration("wait_for_sequencing", time.Second*60, "How long to wait for leaves to be sequenced")
var waitBetweenQueueChecksFlag = flag.Duration("queue_poll_wait", time.Second*5, "How frequently to check the queue while waiting")
var rpcRequestDeadlineFlag = flag.Duration("rpc_deadline", time.Second*10, "Deadline to use for all RPC requests")

func TestLogIntegration(t *testing.T) {
	flag.Parse()
	if *treeIDFlag == -1 {
		t.Skip("Log integration test skipped as no tree ID provided")
	}

	// Initialize and connect to log server
	params := TestParameters{
		treeID:              *treeIDFlag,
		checkLogEmpty:       *checkLogEmptyFlag,
		queueLeaves:         *queueLeavesFlag,
		awaitSequencing:     *awaitSequencingFlag,
		startLeaf:           *startLeafFlag,
		leafCount:           *numLeavesFlag,
		queueBatchSize:      *queueBatchSizeFlag,
		sequencerBatchSize:  *sequencerBatchSizeFlag,
		readBatchSize:       *readBatchSizeFlag,
		sequencingWaitTotal: *waitForSequencingFlag,
		sequencingPollWait:  *waitBetweenQueueChecksFlag,
		rpcRequestDeadline:  *rpcRequestDeadlineFlag,
	}
	if params.startLeaf < 0 || params.leafCount <= 0 {
		t.Fatalf("Start leaf index must be >= 0 (%d) and number of leaves must be > 0 (%d)", params.startLeaf, params.leafCount)
	}

	// TODO: Other options apart from insecure connections
	conn, err := grpc.Dial(*serverFlag, grpc.WithInsecure(), grpc.WithTimeout(time.Second*5))
	if err != nil {
		t.Fatalf("Failed to connect to log server: %v", err)
	}
	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)
	if err := RunLogIntegration(client, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

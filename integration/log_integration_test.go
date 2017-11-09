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

package integration

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/google/trillian"
	_ "github.com/google/trillian/crypto/keys/der/proto" // Register PrivateKey ProtoHandler
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage/memory"
	"github.com/google/trillian/testonly/integration"
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
var customLeafPrefixFlag = flag.String("custom_leaf_prefix", "", "Prefix string added to all queued leaves")

func TestLiveLogIntegration(t *testing.T) {
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
		customLeafPrefix:    *customLeafPrefixFlag,
	}
	if params.startLeaf < 0 || params.leafCount <= 0 {
		t.Fatalf("Start leaf index must be >= 0 (%d) and number of leaves must be > 0 (%d)", params.startLeaf, params.leafCount)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// TODO: Other options apart from insecure connections
	conn, err := grpc.DialContext(ctx, *serverFlag, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to connect to log server: %v", err)
	}
	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)
	if err := RunLogIntegration(client, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestInProcessLogIntegration(t *testing.T) {
	ctx := context.Background()
	const numSequencers = 2
	env, err := integration.NewLogEnv(ctx, numSequencers, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client := trillian.NewTrillianLogClient(env.ClientConn)
	params := DefaultTestParameters(logID)
	if err := RunLogIntegration(client, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestInProcessLogIntegrationDuplicateLeaves(t *testing.T) {
	ctx := context.Background()
	const numSequencers = 2
	ms := memory.NewLogStorage(nil)

	reggie := extension.Registry{
		AdminStorage: memory.NewAdminStorage(ms),
		LogStorage:   ms,
		QuotaManager: quota.Noop(),
	}

	env, err := integration.NewLogEnvWithRegistry(ctx, numSequencers, reggie)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client := trillian.NewTrillianLogClient(env.ClientConn)
	params := DefaultTestParameters(logID)
	params.uniqueLeaves = 10
	if err := RunLogIntegration(client, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

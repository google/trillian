// Copyright 2016 Google LLC. All Rights Reserved.
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

	"google.golang.org/grpc"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage/memory"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly/integration"

	_ "github.com/google/trillian/crypto/keys/der/proto" // Register PrivateKey ProtoHandler
	stestonly "github.com/google/trillian/storage/testonly"
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
		TreeID:              *treeIDFlag,
		CheckLogEmpty:       *checkLogEmptyFlag,
		QueueLeaves:         *queueLeavesFlag,
		AwaitSequencing:     *awaitSequencingFlag,
		StartLeaf:           *startLeafFlag,
		LeafCount:           *numLeavesFlag,
		QueueBatchSize:      *queueBatchSizeFlag,
		SequencerBatchSize:  *sequencerBatchSizeFlag,
		ReadBatchSize:       *readBatchSizeFlag,
		SequencingWaitTotal: *waitForSequencingFlag,
		SequencingPollWait:  *waitBetweenQueueChecksFlag,
		RPCRequestDeadline:  *rpcRequestDeadlineFlag,
		CustomLeafPrefix:    *customLeafPrefixFlag,
	}
	if params.StartLeaf < 0 || params.LeafCount <= 0 {
		t.Fatalf("Start leaf index must be >= 0 (%d) and number of leaves must be > 0 (%d)", params.StartLeaf, params.LeafCount)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// TODO: Other options apart from insecure connections
	conn, err := grpc.DialContext(ctx, *serverFlag, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to connect to log server: %v", err)
	}
	defer conn.Close()

	lc := trillian.NewTrillianLogClient(conn)
	if err := RunLogIntegration(lc, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestInProcessLogIntegration(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	const numSequencers = 2
	env, err := integration.NewLogEnvWithGRPCOptions(ctx, numSequencers, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	tree, err := client.CreateAndInitTree(ctx, &trillian.CreateTreeRequest{
		Tree: stestonly.LogTree,
	}, env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	params := DefaultTestParameters(tree.TreeId)
	if err := RunLogIntegration(env.Log, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestInProcessLogIntegrationDuplicateLeaves(t *testing.T) {
	ctx := context.Background()
	const numSequencers = 2
	ts := memory.NewTreeStorage()
	ms := memory.NewLogStorage(ts, nil)

	reggie := extension.Registry{
		AdminStorage: memory.NewAdminStorage(ts),
		LogStorage:   ms,
		QuotaManager: quota.Noop(),
	}

	env, err := integration.NewLogEnvWithRegistry(ctx, numSequencers, reggie)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	tree, err := client.CreateAndInitTree(ctx, &trillian.CreateTreeRequest{
		Tree: stestonly.LogTree,
	}, env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	params := DefaultTestParameters(tree.TreeId)
	params.UniqueLeaves = 10
	if err := RunLogIntegration(env.Log, params); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

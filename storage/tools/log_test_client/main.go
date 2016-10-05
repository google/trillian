package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage/tools"
	"github.com/vektra/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var queueLeavesFlag = flag.Bool("queue_leaves", true, "If true queues leaves, false just reads from the log")
var awaitSequencingFlag = flag.Bool("await_sequencing", true, "If true then waits until log size is at least num_leaves")
var startLeafFlag = flag.Int64("start_leaf", 0, "The first leaf index to use")
var numLeavesFlag = flag.Int64("num_leaves", 100, "The number of leaves to submit and read back")
var queueBatchSizeFlag = flag.Int("queue_batch_size", 50, "Batch size when queueing leaves")
var readBatchSizeFlag = flag.Int("read_batch_size", 50, "Batch size when getting leaves by index")
var waitForSequencingFlag = flag.Duration("wait_for_sequencing", time.Second * 60, "How long to wait for leaves to be sequenced")
var waitBetweenQueueChecksFlag = flag.Duration("queue_poll_wait", time.Second * 5, "How frequently to check the queue while waiting")

type testParameters struct {
	startLeaf           int64
	leafCount           int64
	queueBatchSize      int
	readBatchSize       int
	sequencingWaitTotal time.Duration
	sequencingPollWait  time.Duration
}

func main() {
	flag.Parse()

	// Step 0 - Initialize and connect to log server
	treeId := tools.GetLogIdFromFlagsOrDie()
	params := testParameters{startLeaf: *startLeafFlag, leafCount: *numLeavesFlag, queueBatchSize: *queueBatchSizeFlag, readBatchSize: *readBatchSizeFlag, sequencingWaitTotal: *waitForSequencingFlag, sequencingPollWait: *waitBetweenQueueChecksFlag}
	port := tools.GetLogServerPort()

	if params.startLeaf < 0 || params.leafCount <= 0 {
		glog.Fatalf("Start leaf index must be >= 0 (%d) and number of leaves must be > 0 (%d)", params.startLeaf, params.leafCount)
	}

	// TODO: Other options apart from insecure connections
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithInsecure(), grpc.WithTimeout(time.Second * 5))

	if err != nil {
		glog.Fatalf("Failed to connect to log server: %v", err)
	}

	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	// Step 1 - Queue leaves on server (optional)
	if *queueLeavesFlag {
		glog.Infof("Queueing %d leaves to log server ...", params.leafCount)
		if err := queueLeaves(treeId, client, params); err != nil {
			glog.Fatalf("Failed to queue leaves: %v", err)
		}
	}

	// Step 2 - Wait for queue to drain when server sequences, give up if it doesn't happen (optional)
	if *awaitSequencingFlag {
		glog.Infof("Waiting for log to sequence ...")
		if err = waitForSequencing(treeId, client, params); err != nil {
			glog.Fatalf("Leaves were not sequenced: %v", err)
		}
	}

	// Step 3 - Use get entries to read back what was written, check leaves are correct
	glog.Infof("Reading back leaves from log ...")
	leafMap, err := readbackLogEntries(treeId, client, params)

	if err != nil {
		glog.Fatalf("Could not read back log entries: %v", err)
	}

	// Step 4 - Cross validation between log and memory tree root hashes
	glog.Infof("Checking log STH with our tree ...")
	tree := buildMemoryMerkleTree(leafMap, params)
	if err := checkLogSTHConsistency(treeId, tree, client, params); err != nil {
		glog.Fatalf("Log consistency check failed: %v", err)
	}
}

func queueLeaves(treeId trillian.LogID, client trillian.TrillianLogClient, params testParameters) error {
	leaves := []trillian.LogLeaf{}

	for l := int64(0); l < params.leafCount; l++ {
		// Leaf data based on the sequence number so we can check the hashes
		leafNumber := params.startLeaf + l

		data := []byte(fmt.Sprintf("Leaf %d", leafNumber))
		hash := sha256.Sum256(data)

		entryTimestamp := trillian.SignedEntryTimestamp{
			TimestampNanos: time.Now().UnixNano(),
			LogId:          treeId.LogID,
			Signature: &trillian.DigitallySigned{
				Signature: []byte("dummy"),
			},
		}

		leaf := trillian.LogLeaf{
			Leaf: trillian.Leaf{
				LeafHash:  trillian.Hash(hash[:]),
				LeafValue: data,
				ExtraData: nil,
			},
			SignedEntryTimestamp: entryTimestamp,
			SequenceNumber:       0}
		leaves = append(leaves, leaf)

		if len(leaves) >= params.queueBatchSize || (l + 1) == params.leafCount {
			glog.Infof("Queueing %d leaves ...", len(leaves))

			req := makeQueueLeavesRequest(treeId, leaves)
			ctx := context.Background()
			response, err := client.QueueLeaves(ctx, &req)

			if err != nil {
				return err
			}

			if response.Status == nil || response.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
				return fmt.Errorf("queue leaves failed: %s %d", response.Status.Description, response.Status.StatusCode)
			}

			leaves = leaves[:0] // starting new batch
		}
	}

	return nil
}

func waitForSequencing(treeId trillian.LogID, client trillian.TrillianLogClient, params testParameters) error {
	endTime := time.Now().Add(params.sequencingWaitTotal)

	glog.Infof("Waiting for sequencing until: %v", endTime)

	for endTime.After(time.Now()) {
		req := trillian.GetSequencedLeafCountRequest{LogId: treeId.TreeID}
		ctx := context.Background()
		sequencedLeaves, err := client.GetSequencedLeafCount(ctx, &req)

		if err != nil {
			return err
		}

		glog.Infof("Leaf count: %d", sequencedLeaves.LeafCount)

		if sequencedLeaves.LeafCount >= params.leafCount + params.startLeaf {
			return nil
		}

		glog.Infof("Leaves sequenced: %d. Still waiting ...", sequencedLeaves.LeafCount)

		time.Sleep(params.sequencingPollWait)
	}

	return errors.New("wait time expired")
}

func readbackLogEntries(logId trillian.LogID, client trillian.TrillianLogClient, params testParameters) (map[int64]*trillian.LeafProto, error) {
	currentLeaf := int64(0)
	leafMap := make(map[int64]*trillian.LeafProto)

	// Build a map of all the leaf data we expect to have seen when we've read all the leaves.
	// Have to work with strings because slices can't be map keys. Sigh.
	leafDataPresenceMap := make(map[string]bool)

	for l := int64(0); l < params.leafCount; l++ {
		leafDataPresenceMap[fmt.Sprintf("Leaf %d", l + params.startLeaf)] = true
	}

	// We have to allow for the last batch potentially being a short one
	for currentLeaf < params.leafCount {
		numLeaves := params.leafCount - currentLeaf

		if numLeaves > int64(params.readBatchSize) {
			numLeaves = int64(params.readBatchSize)
		}

		glog.Infof("Reading %d leaves from %d ...", numLeaves, currentLeaf + params.startLeaf)
		req := makeGetLeavesByIndexRequest(logId, currentLeaf + params.startLeaf, numLeaves)
		response, err := client.GetLeavesByIndex(context.Background(), req)

		if err != nil {
			return nil, err
		}

		if response.Status == nil || response.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
			return nil, fmt.Errorf("read leaves failed: %s %d", response.Status.Description, response.Status.StatusCode)
		}

		// Check we got the right leaf count
		if int64(len(response.Leaves)) != numLeaves {
			return nil, fmt.Errorf("expected %d leaves but we only read %d", numLeaves, len(response.Leaves))
		}

		// Check the leaf contents make sense. Can't rely on exact ordering as queue timestamps will be
		// close between batches and identical within batches.
		for l := 0; l < len(response.Leaves); l++ {
			if _, ok := leafMap[response.Leaves[l].LeafIndex]; ok {
				return nil, fmt.Errorf("got duplicate leaf sequence number: %d", response.Leaves[l].LeafIndex)
			}

			leafMap[response.Leaves[l].LeafIndex] = response.Leaves[l]

			// Test for having seen duplicate leaf data - it should all be distinct
			_, ok := leafDataPresenceMap[string(response.Leaves[l].LeafData)]

			if !ok {
				return nil, fmt.Errorf("leaf data duplicated for leaf: %v", response.Leaves[l])
			}

			delete(leafDataPresenceMap, string(response.Leaves[l].LeafData))

			hash := sha256.Sum256(response.Leaves[l].LeafData)

			if !bytes.Equal(hash[:], response.Leaves[l].LeafHash) {
				return nil, fmt.Errorf("leaf hash mismatch expected: %s, got: %s", base64.StdEncoding.EncodeToString(hash[:]), base64.StdEncoding.EncodeToString(response.Leaves[l].LeafHash))
			}
		}

		currentLeaf += int64(params.readBatchSize)
	}

	// By this point we expect to have seen all the leaves so there should be nothing in the map
	if len(leafDataPresenceMap) != 0 {
		return nil, fmt.Errorf("missing leaves from data read back: %v", leafDataPresenceMap)
	}

	return leafMap, nil
}

func checkLogSTHConsistency(logId trillian.LogID, tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params testParameters) error {
	// Check the STH against the hash we got from our tree
	req := trillian.GetLatestSignedLogRootRequest{LogId: logId.TreeID}
	ctx := context.Background()
	resp, err := client.GetLatestSignedLogRoot(ctx, &req)

	if err != nil {
		return err
	}

	if !bytes.Equal(tree.CurrentRoot().Hash(), resp.SignedLogRoot.RootHash) {
		return fmt.Errorf("root hash mismatch expected: %s, got: %s", base64.StdEncoding.EncodeToString(tree.CurrentRoot().Hash()), base64.StdEncoding.EncodeToString(resp.SignedLogRoot.RootHash))
	}

	return nil
}

func makeQueueLeavesRequest(logId trillian.LogID, leaves []trillian.LogLeaf) trillian.QueueLeavesRequest {
	leafProtos := make([]*trillian.LeafProto, 0, len(leaves))

	for l := 0; l < len(leaves); l++ {
		proto := trillian.LeafProto{LeafIndex: leaves[l].SequenceNumber, LeafHash: leaves[l].LeafHash, LeafData: leaves[l].LeafValue, ExtraData: leaves[l].ExtraData}
		leafProtos = append(leafProtos, &proto)
	}

	return trillian.QueueLeavesRequest{LogId: logId.TreeID, Leaves: leafProtos}
}

func makeGetLeavesByIndexRequest(logID trillian.LogID, startLeaf, numLeaves int64) *trillian.GetLeavesByIndexRequest {
	leafIndices := make([]int64, 0, numLeaves)

	for l := int64(0); l < numLeaves; l++ {
		leafIndices = append(leafIndices, l + startLeaf)
	}

	return &trillian.GetLeavesByIndexRequest{LogId: logID.TreeID, LeafIndex: leafIndices}
}

func buildMemoryMerkleTree(leafMap map[int64]*trillian.LeafProto, params testParameters) *merkle.InMemoryMerkleTree {
	merkleTree := merkle.NewInMemoryMerkleTree(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()))

	// We use the leafMap so we're not relying on the order leaves got sequenced in. We need
	// to use the same order for the memory tree to get the same hash.
	for l := params.startLeaf; l < params.leafCount; l++ {
		merkleTree.AddLeaf(leafMap[l].LeafData)
	}

	return merkleTree
}

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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/testonly"
)

// TestParameters bundles up all the settings for a test run
type TestParameters struct {
	treeID              int64
	checkLogEmpty       bool
	queueLeaves         bool
	awaitSequencing     bool
	startLeaf           int64
	leafCount           int64
	queueBatchSize      int
	sequencerBatchSize  int
	readBatchSize       int64
	sequencingWaitTotal time.Duration
	sequencingPollWait  time.Duration
	rpcRequestDeadline  time.Duration
}

// DefaultTestParameters builds a TestParameters object for a normal
// test of the given log.
func DefaultTestParameters(treeID int64) TestParameters {
	return TestParameters{
		treeID:              treeID,
		checkLogEmpty:       true,
		queueLeaves:         true,
		awaitSequencing:     true,
		startLeaf:           0,
		leafCount:           1000,
		queueBatchSize:      50,
		sequencerBatchSize:  100,
		readBatchSize:       50,
		sequencingWaitTotal: 10 * time.Second * 60,
		sequencingPollWait:  time.Second * 5,
		rpcRequestDeadline:  time.Second * 10,
	}
}

type consistencyProofParams struct {
	size1 int64
	size2 int64
}

// inclusionProofTestIndices are the 0 based leaf indices to probe inclusion proofs at.
var inclusionProofTestIndices = []int64{5, 27, 31, 80, 91}

// consistencyProofTestParams are the intervals
// to test proofs at
var consistencyProofTestParams = []consistencyProofParams{{1, 2}, {2, 3}, {1, 3}, {2, 4}}

// consistencyProofBadTestParams are the intervals to probe for consistency proofs, none of
// these should succeed. Zero is not a valid tree size, nor is -1. 10000000 is outside the
// range we'll reasonably queue (multiple of batch size).
var consistencyProofBadTestParams = []consistencyProofParams{{0, 0}, {-1, 0}, {10000000, 10000000}}

// RunLogIntegration runs a log integration test using the given client and test
// parameters.
func RunLogIntegration(client trillian.TrillianLogClient, params TestParameters) error {
	// Step 1 - Optionally check log starts empty then optionally queue leaves on server
	if params.checkLogEmpty {
		glog.Infof("Checking log is empty before starting test")
		resp, err := getLatestSignedLogRoot(client, params)

		if err != nil {
			return fmt.Errorf("failed to get latest log root: %v %v", resp, err)
		}

		if resp.SignedLogRoot.TreeSize > 0 {
			return fmt.Errorf("expected an empty log but got tree head response: %v", resp)
		}
	}

	if params.queueLeaves {
		glog.Infof("Queueing %d leaves to log server ...", params.leafCount)
		if err := queueLeaves(client, params); err != nil {
			return fmt.Errorf("failed to queue leaves: %v", err)
		}
	}

	// Step 2 - Wait for queue to drain when server sequences, give up if it doesn't happen (optional)
	if params.awaitSequencing {
		glog.Infof("Waiting for log to sequence ...")
		if err := waitForSequencing(params.treeID, client, params); err != nil {
			return fmt.Errorf("leaves were not sequenced: %v", err)
		}
	}

	// Step 3 - Use get entries to read back what was written, check leaves are correct
	glog.Infof("Reading back leaves from log ...")
	leafMap, err := readbackLogEntries(params.treeID, client, params)

	if err != nil {
		return fmt.Errorf("could not read back log entries: %v", err)
	}

	// Step 4 - Cross validation between log and memory tree root hashes
	glog.Infof("Checking log STH with our constructed in-memory tree ...")
	tree := buildMemoryMerkleTree(leafMap, params)
	if err := checkLogRootHashMatches(tree, client, params); err != nil {
		return fmt.Errorf("log consistency check failed: %v", err)
	}

	// Now that the basic tree has passed validation we can start testing proofs

	// Step 5 - Test some inclusion proofs
	glog.Info("Testing inclusion proofs")

	// Ensure log doesn't serve a proof for a leaf index outside the tree size
	if err := checkInclusionProofLeafOutOfRange(params.treeID, client, params); err != nil {
		return fmt.Errorf("log served out of range proof (index): %v", err)
	}

	// Ensure that log doesn't serve a proof for a valid index at a size outside the tree
	if err := checkInclusionProofTreeSizeOutOfRange(params.treeID, client, params); err != nil {
		return fmt.Errorf("log served out of range proof (tree size): %v", err)
	}

	// Probe the log at several leaf indices each with a range of tree sizes
	for _, testIndex := range inclusionProofTestIndices {
		if err := checkInclusionProofsAtIndex(testIndex, params.treeID, tree, client, params); err != nil {
			return fmt.Errorf("log inclusion index: %d proof checks failed: %v", testIndex, err)
		}
	}

	// Step 6 - Test some consistency proofs
	glog.Info("Testing consistency proofs")

	// Make some consistency proof requests that we know should not succeed
	for _, consistParams := range consistencyProofBadTestParams {
		if err := checkConsistencyProof(consistParams, params.treeID, tree, client, params, int64(params.queueBatchSize)); err == nil {
			return fmt.Errorf("log consistency for %v: unexpected proof returned", consistParams)
		}
	}

	// Probe the log between some tree sizes we know are included and check the results against
	// the in memory tree. Request proofs at both STH and non STH sizes unless batch size is one,
	// when these would be equivalent requests.
	for _, consistParams := range consistencyProofTestParams {
		if err := checkConsistencyProof(consistParams, params.treeID, tree, client, params, int64(params.queueBatchSize)); err != nil {
			return fmt.Errorf("log consistency for %v: proof checks failed: %v", consistParams, err)
		}

		// Only do this if the batch size changes when halved
		if params.queueBatchSize > 1 {
			if err := checkConsistencyProof(consistParams, params.treeID, tree, client, params, int64(params.queueBatchSize/2)); err != nil {
				return fmt.Errorf("log consistency for %v: proof checks failed (Non STH size): %v", consistParams, err)
			}
		}
	}
	return nil
}

func queueLeaves(client trillian.TrillianLogClient, params TestParameters) error {
	leaves := []*trillian.LogLeaf{}

	for l := int64(0); l < params.leafCount; l++ {
		// Leaf data based on the sequence number so we can check the hashes
		leafNumber := params.startLeaf + l

		data := []byte(fmt.Sprintf("Leaf %d", leafNumber))
		idHash := sha256.Sum256(data)

		leaf := &trillian.LogLeaf{
			LeafIdentityHash: idHash[:],
			MerkleLeafHash:   testonly.Hasher.HashLeaf(data),
			LeafValue:        data,
			ExtraData:        []byte(fmt.Sprintf("Extra %d", leafNumber)),
		}
		leaves = append(leaves, leaf)

		if len(leaves) >= params.queueBatchSize || (l+1) == params.leafCount {
			glog.Infof("Queueing %d leaves ...", len(leaves))

			ctx, cancel := getRPCDeadlineContext(params)
			_, err := client.QueueLeaves(ctx, &trillian.QueueLeavesRequest{
				LogId:  params.treeID,
				Leaves: leaves,
			})
			cancel()

			if err != nil {
				return err
			}
			leaves = leaves[:0] // starting new batch
		}
	}

	return nil
}

func waitForSequencing(treeID int64, client trillian.TrillianLogClient, params TestParameters) error {
	endTime := time.Now().Add(params.sequencingWaitTotal)

	glog.Infof("Waiting for sequencing until: %v", endTime)

	for endTime.After(time.Now()) {
		req := trillian.GetSequencedLeafCountRequest{LogId: treeID}
		ctx, cancel := getRPCDeadlineContext(params)
		sequencedLeaves, err := client.GetSequencedLeafCount(ctx, &req)
		cancel()

		if err != nil {
			return err
		}

		glog.Infof("Leaf count: %d", sequencedLeaves.LeafCount)

		if sequencedLeaves.LeafCount >= params.leafCount+params.startLeaf {
			return nil
		}

		glog.Infof("Leaves sequenced: %d. Still waiting ...", sequencedLeaves.LeafCount)

		time.Sleep(params.sequencingPollWait)
	}

	return errors.New("wait time expired")
}

func readbackLogEntries(logID int64, client trillian.TrillianLogClient, params TestParameters) (map[int64]*trillian.LogLeaf, error) {
	currentLeaf := int64(0)
	leafMap := make(map[int64]*trillian.LogLeaf)

	// Build a map of all the leaf data we expect to have seen when we've read all the leaves.
	// Have to work with strings because slices can't be map keys. Sigh.
	leafDataPresenceMap := make(map[string]bool)

	for l := int64(0); l < params.leafCount; l++ {
		leafDataPresenceMap[fmt.Sprintf("Leaf %d", l+params.startLeaf)] = true
	}

	for currentLeaf < params.leafCount {
		// We have to allow for the last batch potentially being a short one
		numLeaves := params.leafCount - currentLeaf

		if numLeaves > params.readBatchSize {
			numLeaves = params.readBatchSize
		}

		glog.Infof("Reading %d leaves from %d ...", numLeaves, currentLeaf+params.startLeaf)
		req := makeGetLeavesByIndexRequest(logID, currentLeaf+params.startLeaf, numLeaves)
		ctx, cancel := getRPCDeadlineContext(params)
		response, err := client.GetLeavesByIndex(ctx, req)
		cancel()

		if err != nil {
			return nil, err
		}

		// Check we got the right leaf count
		if len(response.Leaves) == 0 {
			return nil, fmt.Errorf("expected %d leaves log returned none", numLeaves)
		}

		// Check the leaf contents make sense. Can't rely on exact ordering as queue timestamps will be
		// close between batches and identical within batches.
		for l := 0; l < len(response.Leaves); l++ {
			// Check for duplicate leaf index in response data - should not happen
			leaf := response.Leaves[l]

			if _, ok := leafMap[leaf.LeafIndex]; ok {
				return nil, fmt.Errorf("got duplicate leaf sequence number: %d", leaf.LeafIndex)
			}

			leafMap[leaf.LeafIndex] = leaf

			// Test for having seen duplicate leaf data - it should all be distinct
			_, ok := leafDataPresenceMap[string(leaf.LeafValue)]

			if !ok {
				return nil, fmt.Errorf("leaf data duplicated for leaf: %v", leaf)
			}

			delete(leafDataPresenceMap, string(leaf.LeafValue))

			hash := testonly.Hasher.HashLeaf(leaf.LeafValue)

			if got, want := hex.EncodeToString(hash), hex.EncodeToString(leaf.MerkleLeafHash); got != want {
				return nil, fmt.Errorf("leaf %d hash mismatch expected got: %s want: %s", leaf.LeafIndex, got, want)
			}

			// Ensure that the ExtraData in the leaf made it through the roundtrip. This was set up when
			// we queued the leaves.
			if got, want := hex.EncodeToString(leaf.ExtraData), hex.EncodeToString([]byte(strings.Replace(string(leaf.LeafValue), "Leaf", "Extra", 1))); got != want {
				return nil, fmt.Errorf("leaf %d extra data got: %s, want:%s (%v)", leaf.LeafIndex, got, want, leaf)
			}
		}

		currentLeaf += int64(len(response.Leaves))
	}

	// By this point we expect to have seen all the leaves so there should be nothing in the map
	if len(leafDataPresenceMap) != 0 {
		return nil, fmt.Errorf("missing leaves from data read back: %v", leafDataPresenceMap)
	}

	return leafMap, nil
}

func checkLogRootHashMatches(tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params TestParameters) error {
	// Check the STH against the hash we got from our tree
	resp, err := getLatestSignedLogRoot(client, params)

	if err != nil {
		return err
	}

	// Hash must not be empty and must match the one we built ourselves
	if got, want := hex.EncodeToString(resp.SignedLogRoot.RootHash), hex.EncodeToString(tree.CurrentRoot().Hash()); got != want {
		return fmt.Errorf("root hash mismatch expected got: %s want: %s", got, want)
	}

	return nil
}

// checkInclusionProofLeafOutOfRange requests an inclusion proof beyond the current tree size. This
// should fail
func checkInclusionProofLeafOutOfRange(logID int64, client trillian.TrillianLogClient, params TestParameters) error {
	// Test is a leaf index bigger than the current tree size
	ctx, cancel := getRPCDeadlineContext(params)
	proof, err := client.GetInclusionProof(ctx, &trillian.GetInclusionProofRequest{LogId: logID, LeafIndex: params.leafCount + 1, TreeSize: int64(params.leafCount)})
	cancel()

	if err == nil {
		return fmt.Errorf("log returned proof for leaf index outside tree: %d v %d: %v", params.leafCount+1, params.leafCount, proof)
	}

	return nil
}

// checkInclusionProofTreeSizeOutOfRange requests an inclusion proof for a leaf within the tree size at
// a tree size larger than the current tree size. This should fail.
func checkInclusionProofTreeSizeOutOfRange(logID int64, client trillian.TrillianLogClient, params TestParameters) error {
	// Test is an in range leaf index for a tree size that doesn't exist
	ctx, cancel := getRPCDeadlineContext(params)
	proof, err := client.GetInclusionProof(ctx, &trillian.GetInclusionProofRequest{LogId: logID, LeafIndex: int64(params.sequencerBatchSize), TreeSize: params.leafCount + int64(params.sequencerBatchSize)})
	cancel()

	if err == nil {
		return fmt.Errorf("log returned proof for tree size outside tree: %d v %d: %v", params.sequencerBatchSize, params.leafCount+int64(params.sequencerBatchSize), proof)
	}
	return nil
}

// checkInclusionProofsAtIndex obtains and checks proofs at tree sizes from zero up to 2 x the sequencing
// batch size (or number of leaves queued if less). The log should only serve proofs for indices in a tree
// at least as big as the index where STHs where the index is a multiple of the sequencer batch size. All
// proofs returned should match ones computed by the alternate Merkle Tree implementation, which differs
// from what the log uses.
func checkInclusionProofsAtIndex(index int64, logID int64, tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params TestParameters) error {
	for treeSize := int64(0); treeSize < min(params.leafCount, int64(2*params.sequencerBatchSize)); treeSize++ {
		ctx, cancel := getRPCDeadlineContext(params)
		resp, err := client.GetInclusionProof(ctx, &trillian.GetInclusionProofRequest{
			LogId:     logID,
			LeafIndex: index,
			TreeSize:  int64(treeSize),
		})
		cancel()

		// If the index is larger than the tree size we cannot have a valid proof
		if index >= treeSize {
			if err == nil {
				return fmt.Errorf("log returned proof for index: %d, tree is only size %d", index, treeSize)
			}

			continue
		}

		// Otherwise we should have a proof, to be compared against our memory tree
		if err != nil {
			return fmt.Errorf("log returned no proof for index %d at size %d, which should have succeeded: %v", index, treeSize, err)
		}

		// Remember that the in memory tree uses 1 based leaf indices
		path := tree.PathToRootAtSnapshot(index+1, treeSize)

		if err = compareLogAndTreeProof(resp.Proof, path); err != nil {
			// The log and tree proof don't match, details in the error
			return err
		}
	}

	return nil
}

func checkConsistencyProof(consistParams consistencyProofParams, treeID int64, tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params TestParameters, batchSize int64) error {
	// We expect the proof request to succeed
	ctx, cancel := getRPCDeadlineContext(params)
	resp, err := client.GetConsistencyProof(ctx,
		&trillian.GetConsistencyProofRequest{
			LogId:          treeID,
			FirstTreeSize:  consistParams.size1 * int64(batchSize),
			SecondTreeSize: (consistParams.size2 * int64(batchSize)),
		})
	cancel()

	if err != nil {
		return fmt.Errorf("GetConsistencyProof(%v) = %v %v", consistParams, err, resp)
	}

	// Get the proof from the memory tree
	proof := tree.SnapshotConsistency(
		(consistParams.size1 * int64(batchSize)),
		(consistParams.size2 * int64(batchSize)))

	// Compare the proofs, they should be identical
	return compareLogAndTreeProof(resp.Proof, proof)
}

func makeGetLeavesByIndexRequest(logID int64, startLeaf, numLeaves int64) *trillian.GetLeavesByIndexRequest {
	leafIndices := make([]int64, 0, numLeaves)

	for l := int64(0); l < numLeaves; l++ {
		leafIndices = append(leafIndices, l+startLeaf)
	}

	return &trillian.GetLeavesByIndexRequest{LogId: logID, LeafIndex: leafIndices}
}

func buildMemoryMerkleTree(leafMap map[int64]*trillian.LogLeaf, params TestParameters) *merkle.InMemoryMerkleTree {
	// Build the same tree with two different Merkle implementations as an additional check. We don't
	// just rely on the compact tree as the server uses the same code so bugs could be masked
	compactTree := merkle.NewCompactMerkleTree(testonly.Hasher)
	merkleTree := merkle.NewInMemoryMerkleTree(testonly.Hasher)

	// We use the leafMap as we need to use the same order for the memory tree to get the same hash.
	for l := params.startLeaf; l < params.leafCount; l++ {
		compactTree.AddLeaf(leafMap[l].LeafValue, func(depth int, index int64, hash []byte) {})
		merkleTree.AddLeaf(leafMap[l].LeafValue)
	}

	// If the two reference results disagree there's no point in continuing the checks. This is a
	// "can't happen" situation.
	if !bytes.Equal(compactTree.CurrentRoot(), merkleTree.CurrentRoot().Hash()) {
		glog.Fatalf("different root hash results from merkle tree building: %v and %v", compactTree.CurrentRoot(), merkleTree.CurrentRoot())
	}

	return merkleTree
}

func getLatestSignedLogRoot(client trillian.TrillianLogClient, params TestParameters) (*trillian.GetLatestSignedLogRootResponse, error) {
	req := trillian.GetLatestSignedLogRootRequest{LogId: params.treeID}
	ctx, cancel := getRPCDeadlineContext(params)
	resp, err := client.GetLatestSignedLogRoot(ctx, &req)
	cancel()

	return resp, err
}

// getRPCDeadlineTime calculates the future time an RPC should expire based on our config
func getRPCDeadlineContext(params TestParameters) (context.Context, context.CancelFunc) {
	return context.WithDeadline(context.Background(), time.Now().Add(params.rpcRequestDeadline))
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

// compareLogAndTreeProof compares a proof received from the log against one generated by
// an in memory Merkle tree. It ensures the proofs contain an identical list of node hashes.
func compareLogAndTreeProof(logProof *trillian.Proof, treeProof []merkle.TreeEntryDescriptor) error {
	// Compare the proof lengths
	if got, want := len(logProof.GetProofNode()), len(treeProof); got != want {
		return fmt.Errorf("proof differs in length: got: %d want: %d (%s %s)", got, want, formatLogProof(logProof), formatTreeProof(treeProof))
	}

	// Then the node hashes should all match
	for i := 0; i < len(treeProof); i++ {
		if got, want := hex.EncodeToString(logProof.GetProofNode()[i].NodeHash), hex.EncodeToString(treeProof[i].Value.Hash()); got != want {
			return fmt.Errorf("proof mismatch i:%d got: %v want: %v (%s %s)", i, got, want, formatLogProof(logProof), formatTreeProof(treeProof))
		}
	}

	return nil
}

// formatLogProof makes a printable string from a Proof proto
func formatLogProof(proof *trillian.Proof) string {
	hashes := []string{}

	for _, node := range proof.ProofNode {
		hashes = append(hashes, hex.EncodeToString(node.NodeHash))
	}

	return fmt.Sprintf("{ %s }", strings.Join(hashes, ","))
}

// formatTreeProof makes a printable string from a Merkle tree proof
func formatTreeProof(proof []merkle.TreeEntryDescriptor) string {
	hashes := []string{}

	for _, node := range proof {
		hashes = append(hashes, hex.EncodeToString(node.Value.Hash()))
	}

	return fmt.Sprintf("{ %s }", strings.Join(hashes, ","))
}

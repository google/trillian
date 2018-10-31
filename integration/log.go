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
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client/backoff"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/types"
)

// TestParameters bundles up all the settings for a test run
type TestParameters struct {
	treeID              int64
	checkLogEmpty       bool
	queueLeaves         bool
	awaitSequencing     bool
	startLeaf           int64
	leafCount           int64
	uniqueLeaves        int64
	queueBatchSize      int
	sequencerBatchSize  int
	readBatchSize       int64
	sequencingWaitTotal time.Duration
	sequencingPollWait  time.Duration
	rpcRequestDeadline  time.Duration
	customLeafPrefix    string
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
		uniqueLeaves:        1000,
		queueBatchSize:      50,
		sequencerBatchSize:  100,
		readBatchSize:       50,
		sequencingWaitTotal: 10 * time.Second * 60,
		sequencingPollWait:  time.Second * 5,
		rpcRequestDeadline:  time.Second * 30,
		customLeafPrefix:    "",
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

		// TODO(gbelvin): Replace with VerifySignedLogRoot
		var root types.LogRootV1
		if err := root.UnmarshalBinary(resp.SignedLogRoot.GetLogRoot()); err != nil {
			return fmt.Errorf("could not read current log root: %v", err)
		}

		if root.TreeSize > 0 {
			return fmt.Errorf("expected an empty log but got tree head response: %v", resp)
		}
	}

	var leafCounts map[string]int
	var err error
	if params.queueLeaves {
		glog.Infof("Queueing %d leaves to log server ...", params.leafCount)
		if leafCounts, err = queueLeaves(client, params); err != nil {
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
	leafMap, err := readbackLogEntries(params.treeID, client, params, leafCounts)

	if err != nil {
		return fmt.Errorf("could not read back log entries: %v", err)
	}

	// Step 4 - Cross validation between log and memory tree root hashes
	glog.Infof("Checking log STH with our constructed in-memory tree ...")
	tree, err := buildMemoryMerkleTree(leafMap, params)
	if err != nil {
		return err
	}
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

	// TODO(al): test some inclusion proofs by Merkle hash too.

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

func queueLeaves(client trillian.TrillianLogClient, params TestParameters) (map[string]int, error) {
	if params.uniqueLeaves == 0 {
		params.uniqueLeaves = params.leafCount
	}

	leaves := []*trillian.LogLeaf{}

	uniqueLeaves := make([]*trillian.LogLeaf, 0, params.uniqueLeaves)
	for i := int64(0); i < params.uniqueLeaves; i++ {
		leafNumber := params.startLeaf + i
		data := []byte(fmt.Sprintf("%sLeaf %d", params.customLeafPrefix, leafNumber))
		leaf := &trillian.LogLeaf{
			LeafValue: data,
			ExtraData: []byte(fmt.Sprintf("%sExtra %d", params.customLeafPrefix, leafNumber)),
		}
		uniqueLeaves = append(uniqueLeaves, leaf)
	}

	// We'll shuffle the sent leaves around a bit to see if that breaks things,
	// but record and log the seed we use so we can reproduce failures.
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	perm := rand.Perm(int(params.leafCount))
	glog.Infof("Queueing %d leaves total, built from %d unique leaves, using permutation seed %d", params.leafCount, len(uniqueLeaves), seed)

	counts := make(map[string]int)
	for l := int64(0); l < params.leafCount; l++ {
		leaf := uniqueLeaves[int64(perm[l])%params.uniqueLeaves]
		leaves = append(leaves, leaf)
		counts[string(leaf.LeafValue)]++

		if len(leaves) >= params.queueBatchSize || (l+1) == params.leafCount {
			glog.Infof("Queueing %d leaves...", len(leaves))

			ctx, cancel := getRPCDeadlineContext(params)
			b := &backoff.Backoff{
				Min:    100 * time.Millisecond,
				Max:    10 * time.Second,
				Factor: 2,
				Jitter: true,
			}

			err := b.Retry(ctx, func() error {
				_, err := client.QueueLeaves(ctx, &trillian.QueueLeavesRequest{
					LogId:  params.treeID,
					Leaves: leaves,
				})
				return err
			})
			cancel()

			if err != nil {
				return nil, err
			}
			leaves = leaves[:0] // starting new batch
		}
	}

	return counts, nil
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

func readbackLogEntries(logID int64, client trillian.TrillianLogClient, params TestParameters, expect map[string]int) (map[int64]*trillian.LogLeaf, error) {
	// Take a copy of the expect map, since we'll be modifying it:
	expect = func(m map[string]int) map[string]int {
		r := make(map[string]int)
		for k, v := range m {
			r[k] = v
		}
		return r
	}(expect)

	currentLeaf := int64(0)
	leafMap := make(map[int64]*trillian.LogLeaf)
	glog.Infof("Expecting %d unique leaves", len(expect))

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

			lk := string(leaf.LeafValue)
			expect[lk]--

			if expect[lk] == 0 {
				delete(expect, lk)
			}
			leafMap[leaf.LeafIndex] = leaf

			hash, err := rfc6962.DefaultHasher.HashLeaf(leaf.LeafValue)
			if err != nil {
				return nil, fmt.Errorf("HashLeaf(%v): %v", leaf.LeafValue, err)
			}

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
	if len(expect) != 0 {
		return nil, fmt.Errorf("incorrect leaves read back (+missing, -extra): %v", expect)
	}

	return leafMap, nil
}

func checkLogRootHashMatches(tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params TestParameters) error {
	// Check the STH against the hash we got from our tree
	resp, err := getLatestSignedLogRoot(client, params)
	if err != nil {
		return err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(resp.SignedLogRoot.GetLogRoot()); err != nil {
		return err
	}

	// Hash must not be empty and must match the one we built ourselves
	if got, want := root.RootHash, tree.CurrentRoot().Hash(); !bytes.Equal(got, want) {
		return fmt.Errorf("root hash mismatch expected got: %x want: %x", got, want)
	}

	return nil
}

// checkInclusionProofLeafOutOfRange requests an inclusion proof beyond the current tree size. This
// should fail
func checkInclusionProofLeafOutOfRange(logID int64, client trillian.TrillianLogClient, params TestParameters) error {
	// Test is a leaf index bigger than the current tree size
	ctx, cancel := getRPCDeadlineContext(params)
	proof, err := client.GetInclusionProof(ctx, &trillian.GetInclusionProofRequest{
		LogId:     logID,
		LeafIndex: params.leafCount + 1,
		TreeSize:  int64(params.leafCount),
	})
	cancel()

	if err == nil {
		return fmt.Errorf("log returned proof for leaf index outside tree: %d v %d: %v", params.leafCount+1, params.leafCount, proof)
	}

	return nil
}

// checkInclusionProofTreeSizeOutOfRange requests an inclusion proof for a leaf within the tree size at
// a tree size larger than the current tree size. This should succeed but with an STH for the current
// tree and an empty proof, because it is a result of skew.
func checkInclusionProofTreeSizeOutOfRange(logID int64, client trillian.TrillianLogClient, params TestParameters) error {
	// Test is an in range leaf index for a tree size that doesn't exist
	ctx, cancel := getRPCDeadlineContext(params)
	req := &trillian.GetInclusionProofRequest{
		LogId:     logID,
		LeafIndex: int64(params.sequencerBatchSize),
		TreeSize:  params.leafCount + int64(params.sequencerBatchSize),
	}
	proof, err := client.GetInclusionProof(ctx, req)
	cancel()
	if err != nil {
		return fmt.Errorf("log returned error for tree size outside tree: %d v %d: %v", params.leafCount, req.TreeSize, err)
	}

	var root types.LogRootV1
	if err := root.UnmarshalBinary(proof.SignedLogRoot.LogRoot); err != nil {
		return fmt.Errorf("could not read current log root: %v", err)
	}

	if proof.Proof != nil {
		return fmt.Errorf("log returned proof for tree size outside tree: %d v %d: %v", params.leafCount, req.TreeSize, proof)
	}
	if int64(root.TreeSize) >= req.TreeSize {
		return fmt.Errorf("log returned bad root for tree size outside tree: %d v %d: %v", params.leafCount, req.TreeSize, proof)
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
		shouldHaveProof := index < treeSize
		if got, want := err == nil, shouldHaveProof; got != want {
			return fmt.Errorf("GetInclusionProof(index: %d, treeSize %d): %v, want nil: %v", index, treeSize, err, want)
		}
		if !shouldHaveProof {
			continue
		}

		// Verify inclusion proof.
		root := tree.RootAtSnapshot(treeSize).Hash()
		verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)
		// Offset by 1 to make up for C++ / Go implementation differences.
		merkleLeafHash := tree.LeafHash(index + 1)
		if err := verifier.VerifyInclusionProof(index, treeSize, resp.Proof.Hashes, root, merkleLeafHash); err != nil {
			return err
		}
	}

	return nil
}

func checkConsistencyProof(consistParams consistencyProofParams, treeID int64, tree *merkle.InMemoryMerkleTree, client trillian.TrillianLogClient, params TestParameters, batchSize int64) error {
	// We expect the proof request to succeed
	ctx, cancel := getRPCDeadlineContext(params)
	req := &trillian.GetConsistencyProofRequest{
		LogId:          treeID,
		FirstTreeSize:  consistParams.size1 * int64(batchSize),
		SecondTreeSize: consistParams.size2 * int64(batchSize),
	}
	resp, err := client.GetConsistencyProof(ctx, req)
	cancel()
	if err != nil {
		return fmt.Errorf("GetConsistencyProof(%v) = %v %v", consistParams, err, resp)
	}

	if resp.SignedLogRoot == nil || resp.SignedLogRoot.LogRoot == nil {
		return fmt.Errorf("received invalid response: %v", resp)
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
		return fmt.Errorf("could not read current log root: %v", err)
	}

	if req.SecondTreeSize > int64(root.TreeSize) {
		return fmt.Errorf("requested tree size %d > available tree size %d", req.SecondTreeSize, root.TreeSize)
	}

	verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)
	root1 := tree.RootAtSnapshot(req.FirstTreeSize).Hash()
	root2 := tree.RootAtSnapshot(req.SecondTreeSize).Hash()
	return verifier.VerifyConsistencyProof(req.FirstTreeSize, req.SecondTreeSize,
		root1, root2, resp.Proof.Hashes)
}

func makeGetLeavesByIndexRequest(logID int64, startLeaf, numLeaves int64) *trillian.GetLeavesByIndexRequest {
	leafIndices := make([]int64, 0, numLeaves)

	for l := int64(0); l < numLeaves; l++ {
		leafIndices = append(leafIndices, l+startLeaf)
	}

	return &trillian.GetLeavesByIndexRequest{LogId: logID, LeafIndex: leafIndices}
}

func buildMemoryMerkleTree(leafMap map[int64]*trillian.LogLeaf, params TestParameters) (*merkle.InMemoryMerkleTree, error) {
	// Build the same tree with two different Merkle implementations as an additional check. We don't
	// just rely on the compact tree as the server uses the same code so bugs could be masked
	compactTree := compact.NewTree(rfc6962.DefaultHasher)
	merkleTree := merkle.NewInMemoryMerkleTree(rfc6962.DefaultHasher)

	// We use the leafMap as we need to use the same order for the memory tree to get the same hash.
	for l := params.startLeaf; l < params.leafCount; l++ {
		compactTree.AddLeaf(leafMap[l].LeafValue, func(int, int64, []byte) error {
			return nil
		})
		if _, _, err := merkleTree.AddLeaf(leafMap[l].LeafValue); err != nil {
			return nil, err
		}
	}

	// If the two reference results disagree there's no point in continuing the checks. This is a
	// "can't happen" situation.
	if !bytes.Equal(compactTree.CurrentRoot(), merkleTree.CurrentRoot().Hash()) {
		return nil, fmt.Errorf("different root hash results from merkle tree building: %v and %v", compactTree.CurrentRoot(), merkleTree.CurrentRoot())
	}

	return merkleTree, nil
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

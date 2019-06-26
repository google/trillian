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

package server

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// Pass this as a fixed value to proof calculations. It's used as the max depth of the tree
const (
	proofMaxBitLen = 64
	traceSpanRoot  = "/trillian"
)

var (
	optsLogInit            = trees.NewGetOpts(trees.Admin, trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG)
	optsLogRead            = trees.NewGetOpts(trees.Query, trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG)
	optsLogWrite           = trees.NewGetOpts(trees.QueueLog, trillian.TreeType_LOG)
	optsPreorderedLogWrite = trees.NewGetOpts(trees.SequenceLog, trillian.TreeType_PREORDERED_LOG)
)

// TrillianLogRPCServer implements the RPC API defined in the proto
type TrillianLogRPCServer struct {
	registry              extension.Registry
	timeSource            clock.TimeSource
	leafCounter           monitoring.Counter
	proofIndexPercentiles monitoring.Histogram
}

// NewTrillianLogRPCServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogRPCServer(registry extension.Registry, timeSource clock.TimeSource) *TrillianLogRPCServer {
	mf := registry.MetricFactory
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	return &TrillianLogRPCServer{
		registry:   registry,
		timeSource: timeSource,
		leafCounter: mf.NewCounter(
			"queued_leaves",
			"Number of leaves requested to be queued",
			"status",
		),
		proofIndexPercentiles: mf.NewHistogramWithBuckets(
			"proof_index_percentiles",
			"Count of inclusion proof request index using percentage of current log size at the time",
			monitoring.PercentileBuckets(1),
		),
	}
}

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianLogRPCServer) IsHealthy() error {
	ctx, spanEnd := spanFor(context.Background(), "IsHealthy")
	defer spanEnd()
	return t.registry.LogStorage.CheckDatabaseAccessible(ctx)
}

// QueueLeaf submits one leaf to the queue.
func (t *TrillianLogRPCServer) QueueLeaf(ctx context.Context, req *trillian.QueueLeafRequest) (*trillian.QueueLeafResponse, error) {
	ctx, spanEnd := spanFor(ctx, "QueueLeaf")
	defer spanEnd()
	if err := validateLogLeaf(req.Leaf, "QueueLeafRequest.Leaf"); err != nil {
		return nil, err
	}

	queueReq := &trillian.QueueLeavesRequest{
		LogId:  req.LogId,
		Leaves: []*trillian.LogLeaf{req.Leaf},
	}
	queueRsp, err := t.QueueLeaves(ctx, queueReq)
	if err != nil {
		return nil, err
	}
	if queueRsp == nil {
		return nil, status.Errorf(codes.Internal, "missing response")
	}
	if len(queueRsp.QueuedLeaves) != 1 {
		return nil, status.Errorf(codes.Internal, "unexpected count of leaves %d", len(queueRsp.QueuedLeaves))
	}
	return &trillian.QueueLeafResponse{QueuedLeaf: queueRsp.QueuedLeaves[0]}, nil
}

func hashLeaves(leaves []*trillian.LogLeaf, hasher hashers.LogHasher) {
	for _, leaf := range leaves {
		leaf.MerkleLeafHash = hasher.HashLeaf(leaf.LeafValue)
		if len(leaf.LeafIdentityHash) == 0 {
			leaf.LeafIdentityHash = leaf.MerkleLeafHash
		}
	}
}

// QueueLeaves submits a batch of leaves to the log for later integration into the underlying tree.
func (t *TrillianLogRPCServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	ctx, spanEnd := spanFor(ctx, "QueueLeaves")
	defer spanEnd()
	if err := validateLogLeaves(req.Leaves, "QueueLeavesRequest"); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, optsLogWrite)
	if err != nil {
		return nil, err
	}

	ctx = trees.NewContext(ctx, tree)

	hashLeaves(req.Leaves, hasher)

	ret, err := t.registry.LogStorage.QueueLeaves(ctx, tree, req.Leaves, t.timeSource.Now())
	if err != nil {
		return nil, err
	}

	for _, l := range ret {
		if l.Status == nil || l.Status.Code == int32(codes.OK) {
			t.leafCounter.Inc("new")
		} else if l.Status.Code == int32(codes.AlreadyExists) {
			t.leafCounter.Inc("existing")
		}
	}
	return &trillian.QueueLeavesResponse{QueuedLeaves: ret}, nil
}

// AddSequencedLeaf submits one sequenced leaf to the storage.
func (t *TrillianLogRPCServer) AddSequencedLeaf(ctx context.Context, req *trillian.AddSequencedLeafRequest) (*trillian.AddSequencedLeafResponse, error) {
	ctx, spanEnd := spanFor(ctx, "AddSequencedLeaf")
	defer spanEnd()
	if err := validateLogLeaf(req.Leaf, "AddSequencedLeafRequest.Leaf"); err != nil {
		return nil, err
	}

	batchReq := &trillian.AddSequencedLeavesRequest{
		LogId:  req.LogId,
		Leaves: []*trillian.LogLeaf{req.Leaf},
	}
	rsp, err := t.AddSequencedLeaves(ctx, batchReq)
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return nil, status.Errorf(codes.Internal, "missing response")
	}
	if got, want := len(rsp.Results), 1; got != want {
		return nil, status.Errorf(codes.Internal, "expected 1 leaf, got %d", got)
	}
	return &trillian.AddSequencedLeafResponse{Result: rsp.Results[0]}, nil
}

// AddSequencedLeaves submits a batch of sequenced leaves to a pre-ordered log
// for later integration into its underlying tree.
func (t *TrillianLogRPCServer) AddSequencedLeaves(ctx context.Context, req *trillian.AddSequencedLeavesRequest) (*trillian.AddSequencedLeavesResponse, error) {
	ctx, spanEnd := spanFor(ctx, "AddSequencedLeaves")
	defer spanEnd()
	if err := validateAddSequencedLeavesRequest(req); err != nil {
		return nil, err
	}

	tree, hasher, err := t.getTreeAndHasher(ctx, req.LogId, optsPreorderedLogWrite)
	if err != nil {
		return nil, err
	}

	hashLeaves(req.Leaves, hasher)

	ctx = trees.NewContext(ctx, tree)
	leaves, err := t.registry.LogStorage.AddSequencedLeaves(ctx, tree, req.Leaves, t.timeSource.Now())
	if err != nil {
		return nil, err
	}
	if got, want := len(leaves), len(req.Leaves); got != want {
		return nil, status.Errorf(codes.Internal, "AddSequencedLeaves returned %d leaves, want: %d", got, want)
	}

	return &trillian.AddSequencedLeavesResponse{Results: leaves}, nil
}

// GetInclusionProof obtains the proof of inclusion in the tree for a leaf that has been sequenced.
// Similar to the get proof by hash handler but one less step as we don't need to look up the index
func (t *TrillianLogRPCServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetInclusionProof")
	defer spanEnd()
	if err := validateGetInclusionProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.snapshotForTree(ctx, tree, "GetInclusionProof")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetInclusionProof")

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	r := &trillian.GetInclusionProofResponse{SignedLogRoot: slr}

	if uint64(req.TreeSize) > root.TreeSize {
		return r, nil
	}

	proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, req.LeafIndex, int64(root.TreeSize))
	if err != nil {
		return nil, err
	}
	t.recordIndexPercent(req.LeafIndex, root.TreeSize)

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	r.Proof = &proof

	return r, nil
}

// GetInclusionProofByHash obtains proofs of inclusion by leaf hash. Because some logs can
// contain duplicate hashes it is possible for multiple proofs to be returned.
func (t *TrillianLogRPCServer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetInclusionProofByHash")
	defer spanEnd()

	tree, hasher, err := t.getTreeAndHasher(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	if err := validateGetInclusionProofByHashRequest(req, hasher); err != nil {
		return nil, err
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.snapshotForTree(ctx, tree, "GetInclusionProofByHash")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetInclusionProofByHash")

	// Find the leaf index of the supplied hash
	leafHashes := [][]byte{req.LeafHash}
	leaves, err := tx.GetLeavesByHash(ctx, leafHashes, req.OrderBySequence)
	if err != nil {
		return nil, err
	}

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
	proofs := make([]*trillian.Proof, 0, len(leaves))
	for _, leaf := range leaves {
		// Don't include leaves that aren't in the requested TreeSize.
		if leaf.LeafIndex >= req.TreeSize {
			continue
		}
		proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, leaf.LeafIndex, int64(root.TreeSize))
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, &proof)
		t.recordIndexPercent(leaf.LeafIndex, root.TreeSize)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if len(proofs) < 1 {
		return nil, status.Errorf(codes.NotFound,
			"No leaf found for hash: %x in tree size %v", req.LeafHash, req.TreeSize)
	}

	// TODO(gbelvin): Rename "Proof" -> "Proofs"
	return &trillian.GetInclusionProofByHashResponse{
		SignedLogRoot: slr,
		Proof:         proofs,
	}, nil
}

// GetConsistencyProof obtains a proof that two versions of the tree are consistent with each
// other and that the later tree includes all the entries of the prior one. For more details
// see the example trees in RFC 6962.
func (t *TrillianLogRPCServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetConsistencyProof")
	defer spanEnd()
	if err := validateGetConsistencyProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.snapshotForTree(ctx, tree, "GetConsistencyProof")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetConsistencyProof")

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}
	r := &trillian.GetConsistencyProofResponse{SignedLogRoot: slr}

	if uint64(req.SecondTreeSize) > root.TreeSize {
		return r, nil
	}
	// Try to get consistency proof
	proof, err := tryGetConsistencyProof(ctx, req.FirstTreeSize, req.SecondTreeSize, int64(root.TreeSize), tx, hasher)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// We have everything we need. Return the proof
	r.Proof = proof
	return r, nil
}

// GetLatestSignedLogRoot obtains the latest published tree root for the Merkle Tree that
// underlies the log.
func (t *TrillianLogRPCServer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetLatestSignedLogRoot")
	defer spanEnd()
	tree, hasher, err := t.getTreeAndHasher(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)
	tx, err := t.registry.LogStorage.SnapshotForTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetLatestSignedLogRoot")

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	r := &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: slr}

	if req.FirstTreeSize == 0 {
		// no need to get consistency proof in this case
		if err := t.commitAndLog(ctx, req.LogId, tx, "GetLatestSignedLogRoot"); err != nil {
			return nil, err
		}
		return r, nil
	}

	reqProof := &trillian.GetConsistencyProofRequest{
		LogId:          req.LogId,
		FirstTreeSize:  int64(req.FirstTreeSize),
		SecondTreeSize: int64(root.TreeSize),
	}
	if err := validateGetConsistencyProofRequest(reqProof); err != nil {
		return nil, err
	}
	// Try to get consistency proof
	proof, err := tryGetConsistencyProof(ctx, reqProof.FirstTreeSize, reqProof.SecondTreeSize, int64(root.TreeSize), tx, hasher)
	if err != nil {
		return nil, err
	}
	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLatestSignedLogRoot"); err != nil {
		return nil, err
	}
	// We have everything we need. Return the response
	r.Proof = proof
	return r, nil
}

func tryGetConsistencyProof(ctx context.Context, firstTreeSize, secondTreeSize, rootTreeSize int64, tx storage.ReadOnlyLogTreeTX, hasher hashers.LogHasher) (*trillian.Proof, error) {
	nodeFetches, err := merkle.CalcConsistencyProofNodeAddresses(firstTreeSize, secondTreeSize, rootTreeSize, proofMaxBitLen)
	if err != nil {
		return nil, err
	}

	// Do all the node fetches at the second tree revision, which is what the node ids were calculated
	// against.
	rev, err := tx.ReadRevision(ctx)
	if err != nil {
		return nil, err
	}
	proof, err := fetchNodesAndBuildProof(ctx, tx, hasher, rev, 0, nodeFetches)
	if err != nil {
		return nil, err
	}
	return &proof, nil
}

// GetSequencedLeafCount returns the number of leaves that have been integrated into the Merkle
// Tree. This can be zero for a log containing no entries.
func (t *TrillianLogRPCServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetSequencedLeafCount")
	defer spanEnd()
	tree, ctx, err := t.getTreeAndContext(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	tx, err := t.snapshotForTree(ctx, tree, "GetSequencedLeafCount")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetSequencedLeafCount")

	leafCount, err := tx.GetSequencedLeafCount(ctx)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetSequencedLeafCount"); err != nil {
		return nil, err
	}

	return &trillian.GetSequencedLeafCountResponse{LeafCount: leafCount}, nil
}

// GetLeavesByIndex obtains one or more leaves based on their sequence number within the
// tree. It is not possible to fetch leaves that have been queued but not yet integrated.
// TODO: Validate indices against published tree size in case we implement write sharding that
// can get ahead of this point. Not currently clear what component should own this state.
func (t *TrillianLogRPCServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetLeavesByIndex")
	defer spanEnd()
	if err := validateGetLeavesByIndexRequest(req); err != nil {
		return nil, err
	}

	tree, ctx, err := t.getTreeAndContext(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	tx, err := t.snapshotForTree(ctx, tree, "GetLeavesByIndex")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetLeavesByIndex")

	leaves, err := tx.GetLeavesByIndex(ctx, req.LeafIndex)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLeavesByIndex"); err != nil {
		return nil, err
	}

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	return &trillian.GetLeavesByIndexResponse{Leaves: leaves, SignedLogRoot: slr}, nil
}

// GetLeavesByRange obtains leaves based on a range of sequence numbers within the tree.
// This only fetches sequenced leaves; leaves that have been queued but not yet integrated
// are not visible.
func (t *TrillianLogRPCServer) GetLeavesByRange(ctx context.Context, req *trillian.GetLeavesByRangeRequest) (*trillian.GetLeavesByRangeResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetLeavesByRange")
	defer spanEnd()
	if err := validateGetLeavesByRangeRequest(req); err != nil {
		return nil, err
	}

	tree, ctx, err := t.getTreeAndContext(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	tx, err := t.snapshotForTree(ctx, tree, "GetLeavesByRange")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetLeavesByRange")

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	r := &trillian.GetLeavesByRangeResponse{SignedLogRoot: slr}

	if req.StartIndex < int64(root.TreeSize) {
		leaves, err := tx.GetLeavesByRange(ctx, req.StartIndex, req.Count)
		if err != nil {
			return nil, err
		}
		r.Leaves = leaves
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLeavesByRange"); err != nil {
		return nil, err
	}

	return r, nil
}

// GetLeavesByHash obtains one or more leaves based on their tree hash. It is not possible
// to fetch leaves that have been queued but not yet integrated. Logs may accept duplicate
// entries so this may return more results than the number of hashes in the request.
func (t *TrillianLogRPCServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetLeavesByHash")
	defer spanEnd()

	tree, hasher, err := t.getTreeAndHasher(ctx, req.LogId, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	if err := validateGetLeavesByHashRequest(req, hasher); err != nil {
		return nil, err
	}
	tx, err := t.snapshotForTree(ctx, tree, "GetLeavesByHash")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetLeavesByHash")

	leaves, err := tx.GetLeavesByHash(ctx, req.LeafHash, req.OrderBySequence)
	if err != nil {
		return nil, err
	}

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLeavesByHash"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByHashResponse{
		Leaves:        leaves,
		SignedLogRoot: slr,
	}, nil
}

// GetEntryAndProof returns both a Merkle Leaf entry and an inclusion proof for a given index
// and tree size.
func (t *TrillianLogRPCServer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	ctx, spanEnd := spanFor(ctx, "GetEntryAndProof")
	defer spanEnd()
	if err := validateGetEntryAndProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, optsLogRead)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.snapshotForTree(ctx, tree, "GetEntryAndProof")
	if err != nil {
		return nil, err
	}
	defer t.closeAndLog(ctx, tree.TreeId, tx, "GetEntryAndProof")

	slr, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not read current log root: %v", err)
	}

	r := &trillian.GetEntryAndProofResponse{SignedLogRoot: slr}

	if req.TreeSize > int64(root.TreeSize) && req.LeafIndex < int64(root.TreeSize) {
		// return latest proof we can manage
		req.TreeSize = int64(root.TreeSize)
	}

	if req.TreeSize <= int64(root.TreeSize) {
		proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, req.LeafIndex, int64(root.TreeSize))
		if err != nil {
			return nil, err
		}

		// We also need the leaf entry
		leaves, err := tx.GetLeavesByIndex(ctx, []int64{req.LeafIndex})
		if err != nil {
			return nil, err
		}

		if len(leaves) != 1 {
			return nil, status.Errorf(codes.Internal, "expected one leaf from storage but got: %d", len(leaves))
		}

		t.recordIndexPercent(req.LeafIndex, root.TreeSize)

		// Work is complete, we have everything we need for the response
		r.Proof = &proof
		r.Leaf = leaves[0]
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (t *TrillianLogRPCServer) commitAndLog(ctx context.Context, logID int64, tx storage.ReadOnlyLogTreeTX, op string) error {
	err := tx.Commit(ctx)
	if err != nil {
		glog.Warningf("%v: Commit failed for %v: %v", logID, op, err)
	}
	return err
}

func (t *TrillianLogRPCServer) closeAndLog(ctx context.Context, logID int64, tx storage.ReadOnlyLogTreeTX, op string) {
	err := tx.Close()
	if err != nil {
		glog.Warningf("%v: Close failed for %v: %v", logID, op, err)
	}
}

// getInclusionProofForLeafIndex is used by multiple handlers. It does the storage fetching
// and makes additional checks on the returned proof. Returns a Proof suitable for inclusion in
// an RPC response
func getInclusionProofForLeafIndex(ctx context.Context, tx storage.ReadOnlyLogTreeTX, hasher hashers.LogHasher, snapshot, leafIndex, treeSize int64) (trillian.Proof, error) {
	// We have the tree size and leaf index so we know the nodes that we need to serve the proof
	proofNodeIDs, err := merkle.CalcInclusionProofNodeAddresses(snapshot, leafIndex, treeSize, proofMaxBitLen)
	if err != nil {
		return trillian.Proof{}, err
	}

	rev, err := tx.ReadRevision(ctx)
	if err != nil {
		return trillian.Proof{}, err
	}
	return fetchNodesAndBuildProof(ctx, tx, hasher, rev, leafIndex, proofNodeIDs)
}

func (t *TrillianLogRPCServer) getTreeAndHasher(ctx context.Context, treeID int64, opts trees.GetOpts) (*trillian.Tree, hashers.LogHasher, error) {
	tree, err := trees.GetTree(ctx, t.registry.AdminStorage, treeID, opts)
	if err != nil {
		return nil, nil, err
	}
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, err
	}
	return tree, hasher, nil
}

func (t *TrillianLogRPCServer) getTreeAndContext(ctx context.Context, treeID int64, opts trees.GetOpts) (*trillian.Tree, context.Context, error) {
	tree, err := trees.GetTree(ctx, t.registry.AdminStorage, treeID, opts)
	if err != nil {
		return nil, nil, err
	}
	return tree, trees.NewContext(ctx, tree), nil
}

// InitLog initialises a freshly created Log by creating the first STH with
// size 0.
func (t *TrillianLogRPCServer) InitLog(ctx context.Context, req *trillian.InitLogRequest) (*trillian.InitLogResponse, error) {
	ctx, spanEnd := spanFor(ctx, "InitLog")
	defer spanEnd()
	logID := req.LogId
	tree, hasher, err := t.getTreeAndHasher(ctx, logID, optsLogInit)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "getTreeAndHasher()=%v", err)
	}

	var newRoot *trillian.SignedLogRoot
	err = t.registry.LogStorage.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		newRoot = nil

		latestRoot, err := tx.LatestSignedLogRoot(ctx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			return status.Errorf(codes.FailedPrecondition, "LatestSignedLogRoot()=%v", err)
		}

		// Belt and braces check.
		if latestRoot.GetLogRoot() != nil {
			return status.Errorf(codes.AlreadyExists, "log is already initialised")
		}

		signer, err := trees.Signer(ctx, tree)
		if err != nil {
			return status.Errorf(codes.FailedPrecondition, "Signer()=%v", err)
		}

		root, err := signer.SignLogRoot(&types.LogRootV1{
			RootHash:       hasher.EmptyRoot(),
			TimestampNanos: uint64(t.timeSource.Now().UnixNano()),
		})
		if err != nil {
			return err
		}
		newRoot = root

		if err := tx.StoreSignedLogRoot(ctx, newRoot); err != nil {
			return status.Errorf(codes.FailedPrecondition, "StoreSignedLogRoot()=%v", err)
		}

		return nil
	})
	if err != nil && err != storage.ErrTreeNeedsInit {
		return nil, err
	}

	return &trillian.InitLogResponse{
		Created: newRoot,
	}, nil
}

func (t *TrillianLogRPCServer) recordIndexPercent(leafIndex int64, treeSize uint64) {
	if treeSize > 0 {
		// Work out what percentage of the current log size this index corresponds to.
		percent := float64(leafIndex) / float64(treeSize) * 100.0
		t.proofIndexPercentiles.Observe(percent)
	}
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("%s.%s", traceSpanRoot, name))
}

func (t *TrillianLogRPCServer) snapshotForTree(ctx context.Context, tree *trillian.Tree, method string) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := t.registry.LogStorage.SnapshotForTree(ctx, tree)
	if err != nil && tx != nil {
		// Special case to handle ErrTreeNeedsInit, which leaves the TX open.
		// To avoid leaking it make sure it's closed.
		defer t.closeAndLog(ctx, tree.TreeId, tx, method)
	}
	return tx, err
}

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
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// Pass this as a fixed value to proof calculations. It's used as the max depth of the tree
const proofMaxBitLen = 64

// TrillianLogRPCServer implements the RPC API defined in the proto
type TrillianLogRPCServer struct {
	registry    extension.Registry
	timeSource  util.TimeSource
	leafCounter monitoring.Counter
}

// NewTrillianLogRPCServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogRPCServer(registry extension.Registry, timeSource util.TimeSource) *TrillianLogRPCServer {
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
	}
}

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianLogRPCServer) IsHealthy() error {
	return t.registry.LogStorage.CheckDatabaseAccessible(context.Background())
}

// QueueLeaf submits one leaf to the queue.
func (t *TrillianLogRPCServer) QueueLeaf(ctx context.Context, req *trillian.QueueLeafRequest) (*trillian.QueueLeafResponse, error) {
	if err := validateQueueLeafRequest(req); err != nil {
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

// QueueLeaves submits a batch of leaves to the log for later integration into the underlying tree.
func (t *TrillianLogRPCServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	if err := validateQueueLeavesRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, false /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	for _, leaf := range req.Leaves {
		var err error
		leaf.MerkleLeafHash, err = hasher.HashLeaf(leaf.LeafValue)
		if err != nil {
			return nil, err
		}
		if len(leaf.LeafIdentityHash) == 0 {
			leaf.LeafIdentityHash = leaf.MerkleLeafHash
		}
	}

	tx, err := t.prepareStorageTx(ctx, logID)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	existingLeaves, err := tx.QueueLeaves(ctx, req.Leaves, t.timeSource.Now())
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, logID, tx, "QueueLeaves"); err != nil {
		return nil, err
	}

	var queuedLeaves []*trillian.QueuedLogLeaf
	for i, existingLeaf := range existingLeaves {
		if existingLeaf != nil {
			// Append the existing leaf to the response.
			queuedLeaf := trillian.QueuedLogLeaf{
				Leaf:   existingLeaf,
				Status: status.Newf(codes.AlreadyExists, "Leaf already exists: %v", existingLeaf.LeafIdentityHash).Proto(),
			}
			queuedLeaves = append(queuedLeaves, &queuedLeaf)
			t.leafCounter.Inc("existing")
		} else {
			// Return the leaf from the request if it is new.
			queuedLeaf := trillian.QueuedLogLeaf{Leaf: req.Leaves[i]}
			queuedLeaves = append(queuedLeaves, &queuedLeaf)
			t.leafCounter.Inc("new")
		}
	}
	return &trillian.QueueLeavesResponse{QueuedLeaves: queuedLeaves}, nil
}

// GetInclusionProof obtains the proof of inclusion in the tree for a leaf that has been sequenced.
// Similar to the get proof by hash handler but one less step as we don't need to look up the index
func (t *TrillianLogRPCServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	if err := validateGetInclusionProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, req.LeafIndex, root.TreeSize)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &trillian.GetInclusionProofResponse{Proof: &proof}, nil
}

// GetInclusionProofByHash obtains proofs of inclusion by leaf hash. Because some logs can
// contain duplicate hashes it is possible for multiple proofs to be returned.
func (t *TrillianLogRPCServer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	if err := validateGetInclusionProofByHashRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	// Find the leaf index of the supplied hash
	leafHashes := [][]byte{req.LeafHash}
	leaves, err := tx.GetLeavesByHash(ctx, leafHashes, req.OrderBySequence)
	if err != nil {
		return nil, err
	}
	if len(leaves) < 1 {
		return nil, status.Errorf(codes.NotFound, "No leaves for hash: %x", req.LeafHash)
	}

	root, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
	proofs := make([]*trillian.Proof, 0, len(leaves))
	for _, leaf := range leaves {
		proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, leaf.LeafIndex, root.TreeSize)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, &proof)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &trillian.GetInclusionProofByHashResponse{
		Proof: proofs,
	}, nil
}

// GetConsistencyProof obtains a proof that two versions of the tree are consistent with each
// other and that the later tree includes all the entries of the prior one. For more details
// see the example trees in RFC 6962.
func (t *TrillianLogRPCServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	if err := validateGetConsistencyProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.prepareReadOnlyStorageTx(ctx, logID)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	nodeFetches, err := merkle.CalcConsistencyProofNodeAddresses(req.FirstTreeSize, req.SecondTreeSize, root.TreeSize, proofMaxBitLen)
	if err != nil {
		return nil, err
	}

	// Do all the node fetches at the second tree revision, which is what the node ids were calculated
	// against.
	proof, err := fetchNodesAndBuildProof(ctx, tx, hasher, tx.ReadRevision(), 0, nodeFetches)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// We have everything we need. Return the proof
	return &trillian.GetConsistencyProofResponse{Proof: &proof}, nil
}

// GetLatestSignedLogRoot obtains the latest published tree root for the Merkle Tree that
// underlies the log.
func (t *TrillianLogRPCServer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	signedRoot, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLatestSignedLogRoot"); err != nil {
		return nil, err
	}

	return &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: &signedRoot}, nil
}

// GetSequencedLeafCount returns the number of leaves that have been integrated into the Merkle
// Tree. This can be zero for a log containing no entries.
func (t *TrillianLogRPCServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

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
	if err := validateGetLeavesByIndexRequest(req); err != nil {
		return nil, err
	}

	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	leaves, err := tx.GetLeavesByIndex(ctx, req.LeafIndex)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLeavesByIndex"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByIndexResponse{
		Leaves: leaves,
	}, nil
}

// GetLeavesByHash obtains one or more leaves based on their tree hash. It is not possible
// to fetch leaves that have been queued but not yet integrated. Logs may accept duplicate
// entries so this may return more results than the number of hashes in the request.
func (t *TrillianLogRPCServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	if err := validateGetLeavesByHashRequest(req); err != nil {
		return nil, err
	}

	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	leaves, err := tx.GetLeavesByHash(ctx, req.LeafHash, req.OrderBySequence)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, req.LogId, tx, "GetLeavesByHash"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByHashResponse{
		Leaves: leaves,
	}, nil
}

// GetEntryAndProof returns both a Merkle Leaf entry and an inclusion proof for a given index
// and tree size.
func (t *TrillianLogRPCServer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	if err := validateGetEntryAndProofRequest(req); err != nil {
		return nil, err
	}
	logID := req.LogId

	tree, hasher, err := t.getTreeAndHasher(ctx, logID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		return nil, err
	}

	proof, err := getInclusionProofForLeafIndex(ctx, tx, hasher, req.TreeSize, req.LeafIndex, root.TreeSize)
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

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Work is complete, we have everything we need for the response
	return &trillian.GetEntryAndProofResponse{
		Proof: &proof,
		Leaf:  leaves[0],
	}, nil
}

func (t *TrillianLogRPCServer) prepareStorageTx(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	tx, err := t.registry.LogStorage.BeginForTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx, err
}

func (t *TrillianLogRPCServer) prepareReadOnlyStorageTx(ctx context.Context, treeID int64) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := t.registry.LogStorage.SnapshotForTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx, err
}

func (t *TrillianLogRPCServer) commitAndLog(ctx context.Context, logID int64, tx storage.ReadOnlyLogTreeTX, op string) error {
	err := tx.Commit()
	if err != nil {
		glog.Warningf("%v: Commit failed for %v: %v", logID, op, err)
	}
	return err
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

	return fetchNodesAndBuildProof(ctx, tx, hasher, tx.ReadRevision(), leafIndex, proofNodeIDs)
}

func (t *TrillianLogRPCServer) getTreeAndHasher(ctx context.Context, treeID int64, readonly bool) (*trillian.Tree, hashers.LogHasher, error) {
	tree, err := trees.GetTree(
		ctx,
		t.registry.AdminStorage,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: readonly})
	if err != nil {
		return nil, nil, err
	}
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, err
	}
	return tree, hasher, nil
}

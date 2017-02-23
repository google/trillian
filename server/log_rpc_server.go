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
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// Pass this as a fixed value to proof calculations. It's used as the max depth of the tree
const proofMaxBitLen = 64

// TrillianLogRPCServer implements the RPC API defined in the proto
type TrillianLogRPCServer struct {
	registry   extension.Registry
	timeSource util.TimeSource
}

// NewTrillianLogRPCServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogRPCServer(registry extension.Registry, timeSource util.TimeSource) *TrillianLogRPCServer {
	return &TrillianLogRPCServer{
		registry:   registry,
		timeSource: timeSource,
	}
}

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianLogRPCServer) IsHealthy() error {
	s, err := t.registry.GetLogStorage()
	if err != nil {
		return err
	}
	return s.CheckDatabaseAccessible(context.Background())
}

// QueueLeaf submits one leaf to the queue.
func (t *TrillianLogRPCServer) QueueLeaf(ctx context.Context, req *trillian.QueueLeafRequest) (*empty.Empty, error) {
	queueReq := &trillian.QueueLeavesRequest{
		LogId:  req.LogId,
		Leaves: []*trillian.LogLeaf{req.Leaf},
	}
	_, err := t.QueueLeaves(ctx, queueReq)
	if err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// QueueLeaves submits a batch of leaves to the log for later integration into the underlying tree.
func (t *TrillianLogRPCServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if err := validateQueueLeavesRequest(req); err != nil {
		return nil, err
	}

	// TODO(al): Hasher must be selected based on log config.
	th, _ := merkle.Factory(merkle.RFC6962SHA256Type)
	for i := range req.Leaves {
		req.Leaves[i].MerkleLeafHash = th.HashLeaf(req.Leaves[i].LeafValue)
	}

	tx, err := t.prepareStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	err = tx.QueueLeaves(req.Leaves, t.timeSource.Now())
	if err != nil {
		if se, ok := err.(storage.Error); ok && se.ErrType == storage.DuplicateLeaf {
			return nil, grpc.Errorf(codes.AlreadyExists, "Leaf hash already exists: %v", se)
		}

		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "QueueLeaves"); err != nil {
		return nil, err
	}

	return &trillian.QueueLeavesResponse{}, nil
}

// GetInclusionProof obtains the proof of inclusion in the tree for a leaf that has been sequenced.
// Similar to the get proof by hash handler but one less step as we don't need to look up the index
func (t *TrillianLogRPCServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if err := validateGetInclusionProofRequest(req); err != nil {
		return nil, err
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		return nil, err
	}

	proof, err := getInclusionProofForLeafIndex(tx, req.TreeSize, req.LeafIndex, root.TreeSize)
	if err != nil {
		return nil, err
	}

	// The work is complete, can return the response
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &trillian.GetInclusionProofResponse{Proof: &proof}, nil
}

// GetInclusionProofByHash obtains proofs of inclusion by leaf hash. Because some logs can
// contain duplicate hashes it is possible for multiple proofs to be returned.
func (t *TrillianLogRPCServer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if err := validateGetInclusionProofByHashRequest(req); err != nil {
		return nil, err
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	// Find the leaf index of the supplied hash
	leafHashes := [][]byte{req.LeafHash}
	leaves, err := tx.GetLeavesByHash(leafHashes, req.OrderBySequence)
	if err != nil {
		return nil, err
	}
	if len(leaves) < 1 {
		return nil, grpc.Errorf(codes.NotFound, "No leaves for hash: %x", req.LeafHash)
	}

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		return nil, err
	}

	// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
	proofs := make([]*trillian.Proof, 0, len(leaves))
	for _, leaf := range leaves {
		proof, err := getInclusionProofForLeafIndex(tx, req.TreeSize, leaf.LeafIndex, root.TreeSize)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, &proof)
	}

	// The work is complete, can return the response
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
	ctx = util.NewLogContext(ctx, req.LogId)
	if err := validateGetConsistencyProofRequest(req); err != nil {
		return nil, err
	}

	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		return nil, err
	}

	nodeFetches, err := merkle.CalcConsistencyProofNodeAddresses(req.FirstTreeSize, req.SecondTreeSize, root.TreeSize, proofMaxBitLen)
	if err != nil {
		return nil, err
	}

	// Do all the node fetches at the second tree revision, which is what the node ids were calculated
	// against.
	proof, err := fetchNodesAndBuildProof(tx, tx.ReadRevision(), 0, nodeFetches)
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
	ctx = util.NewLogContext(ctx, req.LogId)
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	signedRoot, err := tx.LatestSignedLogRoot()
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetLatestSignedLogRoot"); err != nil {
		return nil, err
	}

	return &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: &signedRoot}, nil
}

// GetSequencedLeafCount returns the number of leaves that have been integrated into the Merkle
// Tree. This can be zero for a log containing no entries.
func (t *TrillianLogRPCServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	leafCount, err := tx.GetSequencedLeafCount()
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetSequencedLeafCount"); err != nil {
		return nil, err
	}

	return &trillian.GetSequencedLeafCountResponse{LeafCount: leafCount}, nil
}

// GetLeavesByIndex obtains one or more leaves based on their sequence number within the
// tree. It is not possible to fetch leaves that have been queued but not yet integrated.
// TODO: Validate indices against published tree size in case we implement write sharding that
// can get ahead of this point. Not currently clear what component should own this state.
func (t *TrillianLogRPCServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if !validateLeafIndices(req.LeafIndex) {
		return &trillian.GetLeavesByIndexResponse{}, nil
	}

	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	leaves, err := tx.GetLeavesByIndex(req.LeafIndex)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetLeavesByIndex"); err != nil {
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
	return t.getLeavesByHashInternal(ctx, "GetLeavesByHash", req, func(tx storage.ReadOnlyLogTreeTX, hashes [][]byte, sequenceOrder bool) ([]*trillian.LogLeaf, error) {
		return tx.GetLeavesByHash(hashes, sequenceOrder)
	})
}

// GetEntryAndProof returns both a Merkle Leaf entry and an inclusion proof for a given index
// and tree size.
func (t *TrillianLogRPCServer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if err := validateGetEntryAndProofRequest(req); err != nil {
		return nil, err
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	root, err := tx.LatestSignedLogRoot()
	if err != nil {
		return nil, err
	}

	proof, err := getInclusionProofForLeafIndex(tx, req.TreeSize, req.LeafIndex, root.TreeSize)
	if err != nil {
		return nil, err
	}

	// We also need the leaf entry
	leaves, err := tx.GetLeavesByIndex([]int64{req.LeafIndex})
	if err != nil {
		return nil, err
	}

	if len(leaves) != 1 {
		return nil, grpc.Errorf(codes.Internal, "expected one leaf from storage but got: %d", len(leaves))
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
	s, err := t.registry.GetLogStorage()
	if err != nil {
		return nil, err
	}
	tx, err := s.BeginForTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx, err
}

func (t *TrillianLogRPCServer) prepareReadOnlyStorageTx(ctx context.Context, treeID int64) (storage.ReadOnlyLogTreeTX, error) {
	s, err := t.registry.GetLogStorage()
	if err != nil {
		return nil, err
	}
	tx, err := s.SnapshotForTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx, err
}

func (t *TrillianLogRPCServer) commitAndLog(ctx context.Context, tx storage.ReadOnlyLogTreeTX, op string) error {
	err := tx.Commit()
	if err != nil {
		glog.Warningf("%s: Commit failed for %s: %v", util.LogIDPrefix(ctx), op, err)
	}
	return err
}

func validateLeafIndices(leafIndices []int64) bool {
	for _, index := range leafIndices {
		if index < 0 {
			return false
		}
	}

	return true
}

// We only validate they're not empty at this point, we let the log do any further checks
func validateLeafHashes(leafHashes [][]byte) bool {
	for _, hash := range leafHashes {
		if len(hash) == 0 {
			return false
		}
	}

	return true
}

// getInclusionProofForLeafIndex is used by multiple handlers. It does the storage fetching
// and makes additional checks on the returned proof. Returns a Proof suitable for inclusion in
// an RPC response
func getInclusionProofForLeafIndex(tx storage.ReadOnlyLogTreeTX, snapshot, leafIndex, treeSize int64) (trillian.Proof, error) {
	// We have the tree size and leaf index so we know the nodes that we need to serve the proof
	proofNodeIDs, err := merkle.CalcInclusionProofNodeAddresses(snapshot, leafIndex, treeSize, proofMaxBitLen)
	if err != nil {
		return trillian.Proof{}, err
	}

	return fetchNodesAndBuildProof(tx, tx.ReadRevision(), leafIndex, proofNodeIDs)
}

// getLeavesByHashInternal does the work of fetching leaves by either their raw data or merkle
// tree hash depending on the supplied fetch function
func (t *TrillianLogRPCServer) getLeavesByHashInternal(ctx context.Context, desc string, req *trillian.GetLeavesByHashRequest, fetchFunc func(storage.ReadOnlyLogTreeTX, [][]byte, bool) ([]*trillian.LogLeaf, error)) (*trillian.GetLeavesByHashResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if len(req.LeafHash) == 0 || !validateLeafHashes(req.LeafHash) {
		return nil, grpc.Errorf(codes.FailedPrecondition, "Invalid leaf hash")
	}

	tx, err := t.prepareReadOnlyStorageTx(ctx, req.LogId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	leaves, err := fetchFunc(tx, req.LeafHash, req.OrderBySequence)
	if err != nil {
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, desc); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByHashResponse{
		Leaves: leaves,
	}, nil
}

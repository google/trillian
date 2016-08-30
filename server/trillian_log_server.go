package server

import (
	"errors"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/merkle"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

var (
	// TODO: Delete when implementation done
	// ErrNotImplemented is returned when an operation is not supported yet
	ErrNotImplemented = errors.New("Not yet implemented")
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// Pass this as a fixed value to proof calculations. It's used as the max depth of the tree
const proofMaxBitLen = 64

// LogStorageProviderFunc decouples the server from storage implementations
type LogStorageProviderFunc func(int64) (storage.LogStorage, error)

// TrillianLogServer implements the RPC API defined in the proto
type TrillianLogServer struct {
	storageProvider LogStorageProviderFunc
	// Must hold this lock before accessing the storage map
	storageMapGuard sync.Mutex
	// Map from tree ID to storage impl for that log
	storageMap map[int64]storage.LogStorage
}

// NewTrillianLogServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogServer(p LogStorageProviderFunc) *TrillianLogServer {
	return &TrillianLogServer{storageProvider: p, storageMap: make(map[int64]storage.LogStorage)}
}

func (t *TrillianLogServer) getStorageForLog(logId int64) (storage.LogStorage, error) {
	t.storageMapGuard.Lock()
	defer t.storageMapGuard.Unlock()

	s, ok := t.storageMap[logId]

	if ok {
		return s, nil
	}

	s, err := t.storageProvider(logId)

	if err != nil {
		t.storageMap[logId] = s
	}
	return s, err
}

// QueueLeaves submits a batch of leaves to the log for later integration into the underlying tree.
func (t *TrillianLogServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	leaves := protosToLeaves(req.Leaves)

	if len(leaves) == 0 {
		return &trillian.QueueLeavesResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Must queue at least one leaf")}, nil
	}

	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	err = tx.QueueLeaves(leaves)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(tx, "QueueLeaves"); err != nil {
		return nil, err
	}

	return &trillian.QueueLeavesResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK)}, nil
}

// GetInclusionProof obtains the proof of inclusion in the tree for a leaf that has been sequenced.
// Similar to the get proof by hash handler but one less step as we don't need to look up the index
func (t *TrillianLogServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	// Reject obviously invalid tree sizes and leaf indices
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("invalid tree size for proof by hash: %d", req.TreeSize)
	}

	if req.LeafIndex <= 0 {
		return nil, fmt.Errorf("invalid leaf index: %d", req.LeafIndex)
	}

	if req.LeafIndex >= req.TreeSize {
		return nil, fmt.Errorf("leaf index %d does not exist in tree of size %d", req.LeafIndex, req.TreeSize)
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	treeRevision, err := tx.GetTreeRevisionAtSize(req.TreeSize)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	proof, err := getInclusionProofForLeafIndexAtRevision(tx, treeRevision, req.TreeSize, req.LeafIndex)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// The work is complete, can return the response
	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	response := trillian.GetInclusionProofResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Proof:&proof}

	return &response, nil
}

// GetInclusionProofByHash obtains proofs of inclusion by leaf hash. Because some logs can
// contain duplicate hashes it is possible for multiple proofs to be returned.
// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
func (t *TrillianLogServer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	// Reject obviously invalid tree sizes
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("invalid tree size for proof by hash: %d", req.TreeSize)
	}

	if len(req.LeafHash) == 0 {
		return nil, fmt.Errorf("invalid leaf hash: %v", req.LeafHash)
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	treeRevision, err := tx.GetTreeRevisionAtSize(req.TreeSize)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// Find the leaf index of the supplied hash
	leafHashes := []trillian.Hash{req.LeafHash}
	leaves, err := tx.GetLeavesByHash(leafHashes, req.OrderBySequence)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): We need to decide how we handle the multiple hash case.
	if len(leaves) != 1 {
		tx.Rollback()
		return nil, fmt.Errorf("expecting one leaf to be returned but got: %d", len(leaves))
	}

	proof, err := getInclusionProofForLeafIndexAtRevision(tx, treeRevision, req.TreeSize, leaves[0].SequenceNumber)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// The work is complete, can return the response
	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	response := trillian.GetInclusionProofByHashResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Proof:[]*trillian.ProofProto{&proof}}

	return &response, nil
}

// GetConsistencyProof obtains a proof that two versions of the tree are consistent with each
// other and that the later tree includes all the entries of the prior one. For more details
// see the example trees in RFC 6962.
func (t *TrillianLogServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	return nil, ErrNotImplemented
}

// GetLatestSignedLogRoot obtains the latest published tree root for the Merkle Tree that
// underlies the log.
func (t *TrillianLogServer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	signedRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(tx, "GetLatestSignedLogRoot"); err != nil {
		return nil, err
	}

	return &trillian.GetLatestSignedLogRootResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), SignedLogRoot: &signedRoot}, nil
}

// GetSequencedLeafCount returns the number of leaves that have been integrated into the Merkle
// Tree. This can be zero for a log containing no entries.
func (t *TrillianLogServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	return nil, ErrNotImplemented
}

// GetLeavesByIndex obtains one or more leaves based on their sequence number within the
// tree. It is not possible to fetch leaves that have been queued but not yet integrated.
// TODO: Validate indices against published tree size in case we implement write sharding that
// can get ahead of this point. Not currently clear what component should own this state.
func (t *TrillianLogServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	if !validateLeafIndices(req.LeafIndex) {
		return &trillian.GetLeavesByIndexResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Invalid -ve leaf index in request")}, nil
	}

	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	leaves, err := tx.GetLeavesByIndex(req.LeafIndex)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	leafProtos := leavesToProtos(leaves)

	if err := t.commitAndLog(tx, "GetLeavesByIndex"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByIndexResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Leaves: leafProtos}, nil
}

// GetLeavesByIndex obtains one or more leaves based on their tree hash. It is not possible
// to fetch leaves that have been queued but not yet integrated. Logs may accept duplicate
// entries so this may return more results than the number of hashes in the request.
func (t *TrillianLogServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	if len(req.LeafHash) == 0 || !validateLeafHashes(req.LeafHash) {
		return &trillian.GetLeavesByHashResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Must supply at least one hash and none must be empty")}, nil
	}

	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	leaves, err := tx.GetLeavesByHash(bytesToHash(req.LeafHash), req.OrderBySequence)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	leafProtos := leavesToProtos(leaves)

	if err := t.commitAndLog(tx, "GetLeavesByHash"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByHashResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Leaves: leafProtos}, nil
}

// GetEntryAndProof returns both a Merkle Leaf entry and an inclusion proof for a given index
// and tree size.
func (t *TrillianLogServer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) prepareStorageTx(treeID int64) (storage.LogTX, error) {
	s, err := t.getStorageForLog(treeID)

	if err != nil {
		return nil, err
	}

	tx, err := s.Begin()

	if err != nil {
		return nil, err
	}

	return tx, err
}

func buildStatus(code trillian.TrillianApiStatusCode) *trillian.TrillianApiStatus {
	return &trillian.TrillianApiStatus{StatusCode: code}
}

func buildStatusWithDesc(code trillian.TrillianApiStatusCode, desc string) *trillian.TrillianApiStatus {
	status := buildStatus(code)
	status.Description = desc

	return status
}

func (t *TrillianLogServer) commitAndLog(tx storage.LogTX, op string) error {
	err := tx.Commit()

	if err != nil {
		glog.Warningf("Commit failed for %s: %v", op, err)
	}

	return err
}

func protoToLeaf(proto *trillian.LeafProto) trillian.LogLeaf {
	return trillian.LogLeaf{SequenceNumber: proto.LeafIndex, Leaf: trillian.Leaf{LeafHash: proto.LeafHash, LeafValue: proto.LeafData, ExtraData: proto.ExtraData}}
}

func protosToLeaves(protos []*trillian.LeafProto) []trillian.LogLeaf {
	leaves := make([]trillian.LogLeaf, 0, len(protos))

	for _, proto := range protos {
		leaves = append(leaves, protoToLeaf(proto))
	}

	return leaves
}

// TODO: Fill in the log leaf specific fields when we've implemented signed timestamps
func leafToProto(leaf trillian.LogLeaf) *trillian.LeafProto {
	return &trillian.LeafProto{LeafIndex: leaf.SequenceNumber, LeafHash: leaf.LeafHash, LeafData: leaf.LeafValue, ExtraData: leaf.ExtraData}
}

func leavesToProtos(leaves []trillian.LogLeaf) []*trillian.LeafProto {
	protos := make([]*trillian.LeafProto, 0, len(leaves))

	for _, leaf := range leaves {
		protos = append(protos, leafToProto(leaf))
	}

	return protos
}

// Don't think we can do this with type assertions, maybe we can
func bytesToHash(inputs [][]byte) []trillian.Hash {
	hashes := make([]trillian.Hash, len(inputs), len(inputs))

	for i, hash := range inputs {
		hashes[i] = trillian.Hash(hash)
	}

	return hashes
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

// getInclusionProofForLeafIndexAtRevision is used by multiple handlers. It does the storage fetching
// and makes additional checks on the returned proof. Returns a ProofProto suitable for inclusion in
// an RPC response
func getInclusionProofForLeafIndexAtRevision(tx storage.LogTX, treeRevision, treeSize, leafIndex int64) (trillian.ProofProto, error) {
	// We have the tree size and leaf index so we know the nodes that we need to serve the proof
	// TODO(Martin2112): Not sure about hardcoding maxBitLen here
	proofNodeIDs, err := merkle.CalcInclusionProofNodeAddresses(treeSize, leafIndex, proofMaxBitLen)

	if err != nil {
		return trillian.ProofProto{}, err
	}

	proofNodes, err := tx.GetMerkleNodes(treeRevision, proofNodeIDs)

	if err != nil {
		return trillian.ProofProto{}, err
	}

	if len(proofNodes) != len(proofNodeIDs) {
		return trillian.ProofProto{}, fmt.Errorf("expected %d nodes in proof but got %d", len(proofNodeIDs), len(proofNodes))
	}

	proof := make([]*trillian.NodeProto, 0, len(proofNodeIDs))

	for i, node := range proofNodes {
		// additional check that the correct node was returned
		if !node.NodeID.Equivalent(proofNodeIDs[i]) {
			return trillian.ProofProto{}, fmt.Errorf("expected node %v at proof pos %d but got %v", proofNodeIDs[i], i, node.NodeID)
		}

		idBytes, err := proto.Marshal(node.NodeID.AsProto())

		if err != nil {
			return trillian.ProofProto{}, err
		}

		proof = append(proof, &trillian.NodeProto{NodeId:idBytes, NodeHash:node.Hash, NodeRevision:node.NodeRevision})
	}

	return trillian.ProofProto{LeafIndex:leafIndex, ProofNode:proof}, nil
}
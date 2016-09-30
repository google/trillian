package server

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
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
	// Signer, so we can sign leaves that we add to logs
	signer crypto.TrillianSigner
	// A timesource so we can create timestamps when signing
	timeSource util.TimeSource
}

// NewTrillianLogServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogServer(p LogStorageProviderFunc, ts crypto.TrillianSigner, timeSource util.TimeSource) *TrillianLogServer {
	return &TrillianLogServer{storageProvider: p, storageMap: make(map[int64]storage.LogStorage), signer: ts, timeSource: timeSource}
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

	// Add the log level signature to the leaves
	for l := 0; l < len(leaves); l++ {
		signature, err := t.signer.Sign(leaves[l].LeafValue)

		if err != nil {
			return nil, err
		}

		// TODO(Martin2112): Sort out log id, not sure it's necessary here and we don't currently
		// have access to it
		leaves[l].SignedEntryTimestamp = trillian.SignedEntryTimestamp{
			Signature:      &signature,
			TimestampNanos: t.timeSource.Now().UnixNano(),
			LogId:          []byte("TODO")}
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

	response := trillian.GetInclusionProofResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Proof: &proof}

	return &response, nil
}

// GetInclusionProofByHash obtains proofs of inclusion by leaf hash. Because some logs can
// contain duplicate hashes it is possible for multiple proofs to be returned.
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

	// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
	proofs := make([]*trillian.ProofProto, 0, len(leaves))

	for _, leaf := range leaves {
		proof, err := getInclusionProofForLeafIndexAtRevision(tx, treeRevision, req.TreeSize, leaf.SequenceNumber)

		if err != nil {
			tx.Rollback()
			return nil, err
		}

		proofs = append(proofs, &proof)
	}

	// The work is complete, can return the response
	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	response := trillian.GetInclusionProofByHashResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Proof: proofs}

	return &response, nil
}

// GetConsistencyProof obtains a proof that two versions of the tree are consistent with each
// other and that the later tree includes all the entries of the prior one. For more details
// see the example trees in RFC 6962.
func (t *TrillianLogServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	// Reject requests where the parameters don't make sense
	if req.FirstTreeSize <= 0 {
		return nil, fmt.Errorf("first tree size must be > 0 but was %d", req.FirstTreeSize)
	}

	if req.SecondTreeSize <= 0 {
		return nil, fmt.Errorf("second tree size must be > 0 but was %d", req.SecondTreeSize)
	}

	if req.SecondTreeSize <= req.FirstTreeSize {
		return nil, fmt.Errorf("second tree size (%d) must be > first tree size (%d)", req.SecondTreeSize, req.FirstTreeSize)
	}

	nodeIDs, err := merkle.CalcConsistencyProofNodeAddresses(req.FirstTreeSize, req.SecondTreeSize, proofMaxBitLen)

	if err != nil {
		return nil, err
	}

	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	// We need to make sure that both the given sizes are actually STHs, though we don't use the
	// first tree revision in fetches
	_, err = tx.GetTreeRevisionAtSize(req.FirstTreeSize)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	secondTreeRevision, err := tx.GetTreeRevisionAtSize(req.SecondTreeSize)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// Do all the node fetches at the second tree revision, which is what the node ids were calculated
	// against.
	proof, err := fetchNodesAndBuildProof(tx, secondTreeRevision, 0, nodeIDs)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	// We have everything we need. Return the proof
	return &trillian.GetConsistencyProofResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Proof: &proof}, nil
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
	tx, err := t.prepareStorageTx(req.LogId)

	if err != nil {
		return nil, err
	}

	leafCount, err := tx.GetSequencedLeafCount()

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(tx, "GetSequencedLeafCount"); err != nil {
		return nil, err
	}

	return &trillian.GetSequencedLeafCountResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), LeafCount: leafCount}, nil
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
	// Reject parameters that are obviously not valid
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("invalid tree size for GetEntryAndProof: %d", req.TreeSize)
	}

	if req.LeafIndex < 0 {
		return nil, fmt.Errorf("invalid params for GetEntryAndProof index: %d", req.LeafIndex)
	}

	if req.LeafIndex >= req.TreeSize {
		return nil, fmt.Errorf("invalid params for GetEntryAndProof index: %d exceeds tree size: %d", req.LeafIndex, req.TreeSize)
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

	// We also need the leaf entry
	leaves, err := tx.GetLeavesByIndex([]int64{req.LeafIndex})

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(leaves) != 1 {
		tx.Rollback()
		return nil, fmt.Errorf("expected one leaf from storage but got: %d", len(leaves))
	}

	leafProtos := leavesToProtos(leaves)

	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	// Work is complete, we have everything we need for the response
	return &trillian.GetEntryAndProofResponse{
		Status: buildStatus(trillian.TrillianApiStatusCode_OK),
		Proof:  &proof,
		Leaf:   leafProtos[0]}, nil
}

func (t *TrillianLogServer) prepareStorageTx(treeID int64) (storage.LogTX, error) {
	s, err := t.storageProvider(treeID)

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

	return fetchNodesAndBuildProof(tx, treeRevision, leafIndex, proofNodeIDs)
}

// fetchNodesAndBuildProof is used by both inclusion and consistency proofs. It fetches the nodes
// from storage and converts them into the proof proto that will be returned to the client.
func fetchNodesAndBuildProof(tx storage.LogTX, treeRevision, leafIndex int64, proofNodeIDs []storage.NodeID) (trillian.ProofProto, error) {
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

		proof = append(proof, &trillian.NodeProto{NodeId: idBytes, NodeHash: node.Hash, NodeRevision: node.NodeRevision})
	}

	return trillian.ProofProto{LeafIndex: leafIndex, ProofNode: proof}, nil
}

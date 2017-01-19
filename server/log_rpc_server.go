package server

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// TODO(Martin2112): Remove this when the feature is fully implemented
var errRehashNotSupported = errors.New("proof request requires rehash but it's not implemented yet")

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

// QueueLeaves submits a batch of leaves to the log for later integration into the underlying tree.
func (t *TrillianLogRPCServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	leaves := depointerify(req.Leaves)

	if len(leaves) == 0 {
		return &trillian.QueueLeavesResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Must queue at least one leaf")}, nil
	}

	// TODO(al): TreeHasher must be selected based on log config.
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	for i := range leaves {
		leaves[i].MerkleLeafHash = th.HashLeaf(leaves[i].LeafValue)
	}

	tx, err := t.prepareStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	err = tx.QueueLeaves(leaves, t.timeSource.Now())
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "QueueLeaves"); err != nil {
		return nil, err
	}

	return &trillian.QueueLeavesResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK)}, nil
}

// GetInclusionProof obtains the proof of inclusion in the tree for a leaf that has been sequenced.
// Similar to the get proof by hash handler but one less step as we don't need to look up the index
func (t *TrillianLogRPCServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	// Reject obviously invalid tree sizes and leaf indices
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("%s: invalid tree size for proof by hash: %d", util.LogIDPrefix(ctx), req.TreeSize)
	}

	if req.LeafIndex <= 0 {
		return nil, fmt.Errorf("%s: invalid leaf index: %d", util.LogIDPrefix(ctx), req.LeafIndex)
	}

	if req.LeafIndex >= req.TreeSize {
		return nil, fmt.Errorf("%s: leaf index %d does not exist in tree of size %d", util.LogIDPrefix(ctx), req.LeafIndex, req.TreeSize)
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	treeRevision, treeSize, err := tx.GetTreeRevisionIncludingSize(req.TreeSize)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Pass tree size as snapshot size to proof recomputation when implemented
	// and remove this check.
	if treeSize != req.TreeSize {
		return nil, errRehashNotSupported
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
func (t *TrillianLogRPCServer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	// Reject obviously invalid tree sizes
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("%s: invalid tree size for proof by hash: %d", util.LogIDPrefix(ctx), req.TreeSize)
	}

	if len(req.LeafHash) == 0 {
		return nil, fmt.Errorf("%s: invalid leaf hash: %v", util.LogIDPrefix(ctx), req.LeafHash)
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	treeRevision, treeSize, err := tx.GetTreeRevisionIncludingSize(req.TreeSize)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Pass tree size as snapshot size to proof recomputation when implemented
	// and remove this check.
	if treeSize != req.TreeSize {
		return nil, errRehashNotSupported
	}

	// Find the leaf index of the supplied hash
	leafHashes := [][]byte{req.LeafHash}
	leaves, err := tx.GetLeavesByHash(leafHashes, req.OrderBySequence)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Need to define a limit on number of results or some form of paging etc.
	proofs := make([]*trillian.Proof, 0, len(leaves))
	for _, leaf := range leaves {
		proof, err := getInclusionProofForLeafIndexAtRevision(tx, treeRevision, req.TreeSize, leaf.LeafIndex)
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
func (t *TrillianLogRPCServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	// Reject requests where the parameters don't make sense
	if req.FirstTreeSize <= 0 {
		return nil, fmt.Errorf("%s: first tree size must be > 0 but was %d", util.LogIDPrefix(ctx), req.FirstTreeSize)
	}

	if req.SecondTreeSize <= 0 {
		return nil, fmt.Errorf("%s: second tree size must be > 0 but was %d", util.LogIDPrefix(ctx), req.SecondTreeSize)
	}

	if req.SecondTreeSize <= req.FirstTreeSize {
		return nil, fmt.Errorf("%s: second tree size (%d) must be > first tree size (%d)", util.LogIDPrefix(ctx), req.SecondTreeSize, req.FirstTreeSize)
	}

	nodeIDs, err := merkle.CalcConsistencyProofNodeAddresses(req.FirstTreeSize, req.SecondTreeSize, proofMaxBitLen)
	if err != nil {
		return nil, err
	}

	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	// We need to make sure that both the given sizes are actually STHs, though we don't use the
	// first tree revision in fetches
	// TODO(Martin2112): This fetch can be removed when rehashing is implemented
	_, firstTreeSize, err := tx.GetTreeRevisionIncludingSize(req.FirstTreeSize)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Pass tree size as snapshot size to proof recomputation when implemented
	// and remove this check.
	if firstTreeSize != req.FirstTreeSize {
		return nil, errRehashNotSupported
	}

	secondTreeRevision, secondTreeSize, err := tx.GetTreeRevisionIncludingSize(req.SecondTreeSize)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Pass tree size as snapshot size to proof recomputation when implemented
	// and remove this check.
	if secondTreeSize != req.SecondTreeSize {
		return nil, errRehashNotSupported
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
func (t *TrillianLogRPCServer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	signedRoot, err := tx.LatestSignedLogRoot()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetLatestSignedLogRoot"); err != nil {
		return nil, err
	}

	return &trillian.GetLatestSignedLogRootResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), SignedLogRoot: &signedRoot}, nil
}

// GetSequencedLeafCount returns the number of leaves that have been integrated into the Merkle
// Tree. This can be zero for a log containing no entries.
func (t *TrillianLogRPCServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	leafCount, err := tx.GetSequencedLeafCount()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetSequencedLeafCount"); err != nil {
		return nil, err
	}

	return &trillian.GetSequencedLeafCountResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), LeafCount: leafCount}, nil
}

// GetLeavesByIndex obtains one or more leaves based on their sequence number within the
// tree. It is not possible to fetch leaves that have been queued but not yet integrated.
// TODO: Validate indices against published tree size in case we implement write sharding that
// can get ahead of this point. Not currently clear what component should own this state.
func (t *TrillianLogRPCServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if !validateLeafIndices(req.LeafIndex) {
		return &trillian.GetLeavesByIndexResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Invalid -ve leaf index in request")}, nil
	}

	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	leaves, err := tx.GetLeavesByIndex(req.LeafIndex)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, "GetLeavesByIndex"); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByIndexResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Leaves: pointerify(leaves)}, nil
}

// GetLeavesByHash obtains one or more leaves based on their tree hash. It is not possible
// to fetch leaves that have been queued but not yet integrated. Logs may accept duplicate
// entries so this may return more results than the number of hashes in the request.
func (t *TrillianLogRPCServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return t.getLeavesByHashInternal(ctx, "GetLeavesByHash", req, func(tx storage.ReadOnlyLogTX, hashes [][]byte, sequenceOrder bool) ([]trillian.LogLeaf, error) {
		return tx.GetLeavesByHash(hashes, sequenceOrder)
	})
}

// GetLeavesByLeafValueHash obtains one or more leaves based on their raw hash. It is not possible
// to fetch leaves that have been queued but not yet integrated. Logs may accept duplicate
// entries so this may return more results than the number of hashes in the request.
func (t *TrillianLogRPCServer) GetLeavesByLeafValueHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return t.getLeavesByHashInternal(ctx, "GetLeavesByLeafValueHash", req, func(tx storage.ReadOnlyLogTX, hashes [][]byte, sequenceOrder bool) ([]trillian.LogLeaf, error) {
		return tx.GetLeavesByLeafValueHash(hashes, sequenceOrder)
	})
}

// GetEntryAndProof returns both a Merkle Leaf entry and an inclusion proof for a given index
// and tree size.
func (t *TrillianLogRPCServer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	// Reject parameters that are obviously not valid
	if req.TreeSize <= 0 {
		return nil, fmt.Errorf("%s: invalid tree size for GetEntryAndProof: %d", util.LogIDPrefix(ctx), req.TreeSize)
	}

	if req.LeafIndex < 0 {
		return nil, fmt.Errorf("%s: invalid params for GetEntryAndProof index: %d", util.LogIDPrefix(ctx), req.LeafIndex)
	}

	if req.LeafIndex >= req.TreeSize {
		return nil, fmt.Errorf("%s: invalid params for GetEntryAndProof index: %d exceeds tree size: %d", util.LogIDPrefix(ctx), req.LeafIndex, req.TreeSize)
	}

	// Next we need to make sure the requested tree size corresponds to an STH, so that we
	// have a usable tree revision
	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	treeRevision, treeSize, err := tx.GetTreeRevisionIncludingSize(req.TreeSize)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// TODO(Martin2112): Pass tree size as snapshot size to proof recomputation when implemented
	// and remove this check.
	if treeSize != req.TreeSize {
		return nil, errRehashNotSupported
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

	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	// Work is complete, we have everything we need for the response
	return &trillian.GetEntryAndProofResponse{
		Status: buildStatus(trillian.TrillianApiStatusCode_OK),
		Proof:  &proof,
		Leaf:   &leaves[0]}, nil
}

func (t *TrillianLogRPCServer) prepareStorageTx(treeID int64) (storage.LogTX, error) {
	s, err := t.registry.GetLogStorage(treeID)
	if err != nil {
		return nil, err
	}

	tx, err := s.Begin()
	if err != nil {
		return nil, err
	}

	return tx, err
}

func (t *TrillianLogRPCServer) prepareReadOnlyStorageTx(treeID int64) (storage.ReadOnlyLogTX, error) {
	s, err := t.registry.GetLogStorage(treeID)
	if err != nil {
		return nil, err
	}

	tx, err := s.Snapshot()
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

func (t *TrillianLogRPCServer) commitAndLog(ctx context.Context, tx storage.ReadOnlyLogTX, op string) error {
	err := tx.Commit()
	if err != nil {
		glog.Warningf("%s: Commit failed for %s: %v", util.LogIDPrefix(ctx), op, err)
	}
	return err
}

func depointerify(protos []*trillian.LogLeaf) []trillian.LogLeaf {
	leaves := make([]trillian.LogLeaf, 0, len(protos))
	for _, leafProto := range protos {
		leaves = append(leaves, *leafProto)
	}
	return leaves
}

func pointerify(leaves []trillian.LogLeaf) []*trillian.LogLeaf {
	protos := make([]*trillian.LogLeaf, 0, len(leaves))
	for _, leaf := range leaves {
		leaf := leaf
		protos = append(protos, &leaf)
	}
	return protos
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
// and makes additional checks on the returned proof. Returns a Proof suitable for inclusion in
// an RPC response
func getInclusionProofForLeafIndexAtRevision(tx storage.ReadOnlyLogTX, treeRevision, treeSize, leafIndex int64) (trillian.Proof, error) {
	// We have the tree size and leaf index so we know the nodes that we need to serve the proof
	// TODO(Martin2112): Not sure about hardcoding maxBitLen here
	proofNodeIDs, err := merkle.CalcInclusionProofNodeAddresses(treeSize, leafIndex, proofMaxBitLen)
	if err != nil {
		return trillian.Proof{}, err
	}

	return fetchNodesAndBuildProof(tx, treeRevision, leafIndex, proofNodeIDs)
}

// getLeavesByHashInternal does the work of fetching leaves by either their raw data or merkle
// tree hash depending on the supplied fetch function
func (t *TrillianLogRPCServer) getLeavesByHashInternal(ctx context.Context, desc string, req *trillian.GetLeavesByHashRequest, fetchFunc func(storage.ReadOnlyLogTX, [][]byte, bool) ([]trillian.LogLeaf, error)) (*trillian.GetLeavesByHashResponse, error) {
	ctx = util.NewLogContext(ctx, req.LogId)
	if len(req.LeafHash) == 0 || !validateLeafHashes(req.LeafHash) {
		return &trillian.GetLeavesByHashResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, fmt.Sprintf("%s: Must supply at least one hash and none must be empty", desc))}, nil
	}

	tx, err := t.prepareReadOnlyStorageTx(req.LogId)
	if err != nil {
		return nil, err
	}

	leaves, err := fetchFunc(tx, req.LeafHash, req.OrderBySequence)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := t.commitAndLog(ctx, tx, desc); err != nil {
		return nil, err
	}

	return &trillian.GetLeavesByHashResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Leaves: pointerify(leaves)}, nil
}

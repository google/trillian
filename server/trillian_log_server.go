package server

import (
	"errors"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
)

var (
	// TODO: Delete when implementation done
	// ErrNotImplemented is returned when an operation is not supported yet
	ErrNotImplemented = errors.New("Not yet implemented")
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

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

func (t *TrillianLogServer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	if !validateLeafIndices(req.LeafIndex) {
		return &trillian.GetLeavesByIndexResponse{Status: buildStatusWithDesc(trillian.TrillianApiStatusCode_ERROR, "Invalid -ve leaf index in request")}, nil
	}

	tx, err := t.prepareStorageTx(*req.LogId)

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
		tx.Rollback()
		return nil, err
	}

	return &trillian.GetLeavesByIndexResponse{Status: buildStatus(trillian.TrillianApiStatusCode_OK), Leaves: leafProtos}, nil
}

func (t *TrillianLogServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
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
	status := code
	return &trillian.TrillianApiStatus{StatusCode: &status}
}

func buildStatusWithDesc(code trillian.TrillianApiStatusCode, desc string) *trillian.TrillianApiStatus {
	status := buildStatus(code)
	status.Description = &desc

	return status
}

func (t *TrillianLogServer) commitAndLog(tx storage.LogTX, op string) error {
	err := tx.Commit()

	if err != nil {
		glog.Warningf("Commit failed for %s: %v", op, err)
	}

	return err
}

// TODO: Fill in the log leaf specific fields when we've implemented signed timestamps
func leafToProto(leaf trillian.LogLeaf) *trillian.LeafProto {
	return &trillian.LeafProto{LeafHash: leaf.LeafHash, LeafData: leaf.LeafValue, ExtraData: leaf.ExtraData}
}

func leavesToProtos(leaves []trillian.LogLeaf) []*trillian.LeafProto {
	protos := make([]*trillian.LeafProto, 0, len(leaves))

	for _, leaf := range leaves {
		protos = append(protos, leafToProto(leaf))
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

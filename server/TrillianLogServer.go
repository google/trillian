package server

import (
	"errors"
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
)

var (
	// TODO: Delete when implementation done
	// ErrNotImplemented is returned when an operation is not supported yet
	ErrNotImplemented = errors.New("Not yet implemented")
)

// LogStorageProviderFunc decouples the server from storage implementations
type LogStorageProviderFunc func(int64) (*storage.LogStorage, error)

// TrillianLogServer implements the RPC API defined in the proto
type TrillianLogServer struct {
	storageProvider LogStorageProviderFunc
	// Must hold this lock before accessing the storage map
	storageMapGuard sync.Mutex
	// Map from tree ID to storage impl for that log
	storageMap map[int64]*storage.LogStorage
}

// NewTrillianLogServer creates a new RPC server backed by a LogStorageProvider.
func NewTrillianLogServer(p LogStorageProviderFunc) *TrillianLogServer {
	return &TrillianLogServer{storageProvider: p}
}

func (t *TrillianLogServer) getStorageForLog(logId int64) (*storage.LogStorage, error) {
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
	return nil, ErrNotImplemented
}

func (t *TrillianLogServer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	return nil, ErrNotImplemented
}

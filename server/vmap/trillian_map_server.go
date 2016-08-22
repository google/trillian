package vmap

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

// MapStorageProviderFunc decouples the server from storage implementations
type MapStorageProviderFunc func(int64) (storage.MapStorage, error)

// TrillianMapServer implements the RPC API defined in the proto
type TrillianMapServer struct {
	storageProvider MapStorageProviderFunc
	// Must hold this lock before accessing the storage map
	storageMapGuard sync.Mutex
	// Map from tree ID to storage impl for that map
	storageMap map[int64]storage.MapStorage
}

// NewTrillianMaperver creates a new RPC server backed by a MapStorageProvider.
func NewTrillianMapServer(p MapStorageProviderFunc) *TrillianMapServer {
	return &TrillianMapServer{storageProvider: p, storageMap: make(map[int64]storage.MapStorage)}
}

func (t *TrillianMapServer) getStorageForMap(mapId int64) (storage.MapStorage, error) {
	t.storageMapGuard.Lock()
	defer t.storageMapGuard.Unlock()

	s, ok := t.storageMap[mapId]

	if ok {
		return s, nil
	}

	s, err := t.storageProvider(mapId)

	if err != nil {
		t.storageMap[mapId] = s
	}
	return s, err
}

// GetLeaves implements the GetLeaves RPC method.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (*trillian.GetMapLeavesResponse, error) {
	return nil, ErrNotImplemented
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (*trillian.SetMapLeavesResponse, error) {
	return nil, ErrNotImplemented
}

// GetSignedMapRoot implements the GetSignedMapRoot RPC method.
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (*trillian.GetSignedMapRootResponse, error) {
	return nil, ErrNotImplemented
}

func buildStatus(code trillian.TrillianApiStatusCode) *trillian.TrillianApiStatus {
	return &trillian.TrillianApiStatus{StatusCode: code}
}

func buildStatusWithDesc(code trillian.TrillianApiStatusCode, desc string) *trillian.TrillianApiStatus {
	status := buildStatus(code)
	status.Description = desc

	return status
}

func (t *TrillianMapServer) commitAndLog(tx storage.MapTX, op string) error {
	err := tx.Commit()

	if err != nil {
		glog.Warningf("Commit failed for %s: %v", op, err)
	}

	return err
}

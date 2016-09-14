package vmap

import (
	"errors"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
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

func (t *TrillianMapServer) getHasherForMap(mapId int64) (merkle.MapHasher, error) {
	// TODO(al): actually return tailored hashers.
	return merkle.NewMapHasher(merkle.NewRFC6962TreeHasher(trillian.NewSHA256())), nil
}

// GetLeaves implements the GetLeaves RPC method.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (resp *trillian.GetMapLeavesResponse, err error) {
	s, err := t.getStorageForMap(req.MapId)
	if err != nil {
		return nil, err
	}

	tx, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	defer func() {
		e := tx.Commit()
		if e != nil && err == nil {
			resp, err = nil, e
		}
	}()

	kh, err := t.getHasherForMap(req.MapId)
	if err != nil {
		return nil, err
	}

	var root *trillian.SignedMapRoot

	if req.Revision < 0 {
		// need to know the newest published revision
		r, err := tx.LatestSignedMapRoot()
		if err != nil {
			return nil, err
		}
		root = &r
		req.Revision = root.MapRevision
	}

	smtReader := merkle.NewSparseMerkleTreeReader(req.Revision, kh, tx)

	resp = &trillian.GetMapLeavesResponse{
		KeyValue: make([]*trillian.KeyValueInclusion, 0, len(req.Key)),
	}

	for i := 0; i < len(req.Key); i++ {
		key := req.Key[i]
		proof, err := smtReader.InclusionProof(req.Revision, key)
		if err != nil {
			return nil, err
		}

		leaf, err := tx.Get(req.Revision, kh.HashKey(key))
		if err != nil {
			return nil, err
		}

		kvi := trillian.KeyValueInclusion{
			KeyValue: &trillian.KeyValue{
				Key:   key,
				Value: &leaf,
			},
			Inclusion: make([][]byte, 0, len(proof)),
		}
		for j := 0; j < len(proof); j++ {
			kvi.Inclusion = append(kvi.Inclusion, []byte(proof[j]))
		}

		resp.KeyValue = append(resp.KeyValue, &kvi)
	}

	return resp, nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (resp *trillian.SetMapLeavesResponse, err error) {
	s, err := t.getStorageForMap(req.MapId)
	if err != nil {
		return nil, err
	}

	tx, err := s.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			// Something went wrong, we should rollback and not return any partial/wrong data
			resp = nil
			tx.Rollback()
			return
		}
		// try to commit the tx
		e := tx.Commit()
		if e != nil {
			// don't return partial/uncommited/wrong data:
			resp = nil
			err = e
		}
	}()

	hasher, err := t.getHasherForMap(req.MapId)
	if err != nil {
		return nil, err
	}

	glog.Infof("Writing at revision %d", tx.WriteRevision())

	smtWriter, err := merkle.NewSparseMerkleTreeWriter(tx.WriteRevision(), hasher, func() (storage.TreeTX, error) {
		return s.Begin()
	})
	if err != nil {
		return nil, err
	}

	leaves := make([]merkle.HashKeyValue, 0, len(req.KeyValue))
	for i := 0; i < len(req.KeyValue); i++ {
		kv := req.KeyValue[i]
		kHash := hasher.HashKey(kv.Key)
		vHash := hasher.HashLeaf(kv.Value.LeafValue)
		leaves = append(leaves, merkle.HashKeyValue{kHash, vHash})
		if err = tx.Set(kHash, *kv.Value); err != nil {
			return nil, err
		}
	}
	if err = smtWriter.SetLeaves(leaves); err != nil {
		return nil, err
	}
	rootHash, err := smtWriter.CalculateRoot()

	newRoot := trillian.SignedMapRoot{
		TimestampNanos: time.Now().UnixNano(),
		RootHash:       rootHash,
		MapId:          s.MapID().MapID,
		MapRevision:    tx.WriteRevision(),
		Metadata:       req.MapperData,
		// TODO(al): Actually sign stuff, etc!
		Signature: &trillian.DigitallySigned{},
	}

	// TODO(al): need an smtWriter.Rollback() or similar I think.
	if err = tx.StoreSignedMapRoot(newRoot); err != nil {
		return nil, err
	}
	resp = &trillian.SetMapLeavesResponse{
		MapRoot: &newRoot,
	}
	return resp, nil
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

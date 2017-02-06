package vmap

import (
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

// TODO: There is no access control in the server yet and clients could easily modify
// any tree.

// TrillianMapServer implements the RPC API defined in the proto
type TrillianMapServer struct {
	registry extension.Registry
}

// NewTrillianMapServer creates a new RPC server backed by registry
func NewTrillianMapServer(registry extension.Registry) *TrillianMapServer {
	return &TrillianMapServer{registry}
}

func (t *TrillianMapServer) getHasherForMap(mapID int64) (merkle.MapHasher, error) {
	// TODO(al): actually return tailored hashers.
	return merkle.NewMapHasher(merkle.NewRFC6962TreeHasher()), nil
}

// GetLeaves implements the GetLeaves RPC method.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (resp *trillian.GetMapLeavesResponse, err error) {
	ctx = util.NewMapContext(ctx, req.MapId)
	s, err := t.registry.GetMapStorage()
	if err != nil {
		return nil, err
	}

	tx, err := s.SnapshotForTree(ctx, req.MapId)
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

	leaves, err := tx.Get(req.Revision, req.Index)
	if err != nil {
		return nil, err
	}

	glog.Infof("%s: wanted %d leaves, found %d", util.MapIDPrefix(ctx), len(req.Index), len(leaves))

	resp = &trillian.GetMapLeavesResponse{
		IndexValueInclusion: make([]*trillian.IndexValueInclusion, len(leaves)),
	}
	for i, leaf := range leaves {
		proof, err := smtReader.InclusionProof(req.Revision, leaf.Index)
		if err != nil {
			return nil, err
		}
		// Copy the leaf from the iterator, which gets overwritten
		value := leaf
		resp.IndexValueInclusion[i] = &trillian.IndexValueInclusion{
			IndexValue: &trillian.IndexValue{
				Index: leaf.Index,
				Value: &value,
			},
			Inclusion: proof,
		}
	}

	return resp, nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (resp *trillian.SetMapLeavesResponse, err error) {
	ctx = util.NewMapContext(ctx, req.MapId)
	s, err := t.registry.GetMapStorage()
	if err != nil {
		return nil, err
	}

	tx, err := s.BeginForTree(ctx, req.MapId)
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
			// don't return partial/uncommitted/wrong data:
			glog.Warningf("%s: Commit failed for SetLeaves: %v", util.MapIDPrefix(ctx), e)
			resp = nil
			err = e
		}
	}()

	hasher, err := t.getHasherForMap(req.MapId)
	if err != nil {
		return nil, err
	}

	glog.Infof("%s: Writing at revision %d", util.MapIDPrefix(ctx), tx.WriteRevision())

	smtWriter, err := merkle.NewSparseMerkleTreeWriter(tx.WriteRevision(), hasher, func() (storage.TreeTX, error) {
		return s.BeginForTree(ctx, req.MapId)
	})
	if err != nil {
		return nil, err
	}

	for _, kv := range req.IndexValue {
		// TODO(gbelvin) use LeafHash rather than computing here.
		kv.Value.LeafHash = hasher.HashLeaf(kv.Value.LeafValue)
		// TODO(gbelvin) only have ONE place where index is stored.
		// TODO(gbelvin) Verify that Index is of the proper length.
		kv.Value.Index = kv.Index

		if err = tx.Set(kv.Value.Index, *kv.Value); err != nil {
			return nil, err
		}
		if err = smtWriter.SetLeaves([]merkle.HashKeyValue{
			{
				HashedKey:   kv.Value.Index,
				HashedValue: kv.Value.LeafHash,
			},
		}); err != nil {
			return nil, err
		}
	}

	rootHash, err := smtWriter.CalculateRoot()
	newRoot := trillian.SignedMapRoot{
		TimestampNanos: time.Now().UnixNano(),
		RootHash:       rootHash,
		MapId:          req.MapId,
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
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (resp *trillian.GetSignedMapRootResponse, err error) {
	ctx = util.NewMapContext(ctx, req.MapId)
	s, err := t.registry.GetMapStorage()
	if err != nil {
		return nil, err
	}

	tx, err := s.SnapshotForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer func() {
		// try to commit the tx
		e := tx.Commit()
		if e != nil && err == nil {
			glog.Warningf("%s: Commit failed for GetSignedMapRoot: %v", util.MapIDPrefix(ctx), e)
			resp, err = nil, e
		}
	}()

	r, err := tx.LatestSignedMapRoot()
	if err != nil {
		return nil, err
	}

	resp = &trillian.GetSignedMapRootResponse{
		MapRoot: &r,
	}
	return resp, err
}

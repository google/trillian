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

package vmap

import (
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
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

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianMapServer) IsHealthy() error {
	return t.registry.MapStorage.CheckDatabaseAccessible(context.Background())
}

// GetLeaves implements the GetLeaves RPC method.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (*trillian.GetMapLeavesResponse, error) {
	mapID := req.GetMapId()

	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)
	ctx = util.NewMapContext(ctx, mapID)

	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, mapID)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

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

	smtReader := merkle.NewSparseMerkleTreeReader(req.Revision, hasher, tx)

	leaves, err := tx.Get(req.Revision, req.Index)
	if err != nil {
		return nil, err
	}
	glog.Infof("%s: wanted %d leaves, found %d", util.MapIDPrefix(ctx), len(req.Index), len(leaves))

	resp := &trillian.GetMapLeavesResponse{
		MapLeafInclusion: make([]*trillian.MapLeafInclusion, len(leaves)),
	}
	for i, leaf := range leaves {
		proof, err := smtReader.InclusionProof(req.Revision, leaf.Index)
		if err != nil {
			return nil, err
		}
		// Copy the leaf from the iterator, which gets overwritten
		value := leaf
		resp.MapLeafInclusion[i] = &trillian.MapLeafInclusion{
			Leaf:      &value,
			Inclusion: proof,
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return resp, nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (*trillian.SetMapLeavesResponse, error) {
	mapID := req.GetMapId()

	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, false /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)
	ctx = util.NewMapContext(ctx, mapID)

	tx, err := t.registry.MapStorage.BeginForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	glog.Infof("%s: Writing at revision %d", util.MapIDPrefix(ctx), tx.WriteRevision())
	smtWriter, err := merkle.NewSparseMerkleTreeWriter(tx.WriteRevision(), hasher, func() (storage.TreeTX, error) {
		return t.registry.MapStorage.BeginForTree(ctx, req.MapId)
	})
	if err != nil {
		return nil, err
	}

	for _, l := range req.Leaves {
		// TODO(gbelvin) Verify that Index is of the proper length.
		// TODO(gbelvin) use LeafHash rather than computing here.
		l.LeafHash = hasher.HashLeaf(l.LeafValue)

		if err = tx.Set(l.Index, *l); err != nil {
			return nil, err
		}
		if err = smtWriter.SetLeaves([]merkle.HashKeyValue{
			{
				HashedKey:   l.Index,
				HashedValue: l.LeafHash,
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
		Signature: &spb.DigitallySigned{},
	}

	// TODO(al): need an smtWriter.Rollback() or similar I think.
	if err = tx.StoreSignedMapRoot(newRoot); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%s: Commit failed for SetLeaves: %v", util.MapIDPrefix(ctx), err)
		return nil, err
	}

	return &trillian.SetMapLeavesResponse{
		MapRoot: &newRoot,
	}, nil
}

// GetSignedMapRoot implements the GetSignedMapRoot RPC method.
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (*trillian.GetSignedMapRootResponse, error) {
	ctx = util.NewMapContext(ctx, req.MapId)
	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	r, err := tx.LatestSignedMapRoot()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%s: Commit failed for GetSignedMapRoot: %v", util.MapIDPrefix(ctx), err)
		return nil, err
	}

	return &trillian.GetSignedMapRootResponse{
		MapRoot: &r,
	}, nil
}

func (t *TrillianMapServer) getTreeAndHasher(ctx context.Context, treeID int64, readonly bool) (*trillian.Tree, merkle.MapHasher, error) {
	tree, err := trees.GetTree(
		ctx,
		t.registry.AdminStorage,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: readonly})
	if err != nil {
		return nil, merkle.MapHasher{}, err
	}
	th, err := trees.Hasher(tree)
	if err != nil {
		return nil, merkle.MapHasher{}, err
	}
	return tree, merkle.NewMapHasher(th), nil
}

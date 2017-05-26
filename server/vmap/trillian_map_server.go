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

	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// indexSize is the number of bytes all indexes should be.
	// TODO(gdbelvin): specify the index length in the tree specification.
	indexSize = 32
)

// TODO(codingllama): There is no access control in the server yet and clients could easily modify
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

// GetLeaves implements the GetLeaves RPC method.  Each requested index will
// return an inclusion proof to either the leaf, or nil if the leaf does not
// exist.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (*trillian.GetMapLeavesResponse, error) {
	mapID := req.MapId

	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, mapID)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	var root *trillian.SignedMapRoot
	if req.Revision < 0 {
		// need to know the newest published revision
		r, err := tx.LatestSignedMapRoot(ctx)
		if err != nil {
			return nil, err
		}
		root = &r
		req.Revision = root.MapRevision
	}

	smtReader := merkle.NewSparseMerkleTreeReader(req.Revision, hasher, tx)

	inclusions := make([]*trillian.MapLeafInclusion, 0, len(req.Index))
	found := 0
	for _, index := range req.Index {
		// TODO(gdbelvin): specify the index length in the tree specification.
		if got, want := len(index), indexSize; got != want {
			return nil, status.Errorf(codes.InvalidArgument,
				"index len(%x): %v, want %v", index, got, want)
		}
		// Fetch the leaf if it exists.
		leaves, err := tx.Get(ctx, req.Revision, [][]byte{index})
		if err != nil {
			return nil, err
		}
		var leaf *trillian.MapLeaf
		if len(leaves) == 1 {
			leaf = &leaves[0]
			found++
		} else {
			// Empty leaf for proof of non-existence.
			leaf = &trillian.MapLeaf{
				Index:     index,
				LeafValue: nil,
			}
		}

		// Fetch the proof regardless of whether the leaf exists.
		proof, err := smtReader.InclusionProof(ctx, req.Revision, index)
		if err != nil {
			return nil, err
		}

		inclusions = append(inclusions, &trillian.MapLeafInclusion{
			Leaf:      leaf,
			Inclusion: proof,
		})
	}
	glog.Infof("%v: wanted %v leaves, found %v", mapID, len(req.Index), found)

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &trillian.GetMapLeavesResponse{
		MapLeafInclusion: inclusions,
	}, nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (*trillian.SetMapLeavesResponse, error) {
	mapID := req.MapId

	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, false /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.registry.MapStorage.BeginForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	glog.V(2).Infof("%v: Writing at revision %v", mapID, tx.WriteRevision())
	smtWriter, err := merkle.NewSparseMerkleTreeWriter(
		ctx,
		tx.WriteRevision(),
		hasher, func() (storage.TreeTX, error) {
			return t.registry.MapStorage.BeginForTree(ctx, req.MapId)
		})
	if err != nil {
		return nil, err
	}

	for _, l := range req.Leaves {
		if got, want := len(l.Index), indexSize; got != want {
			return nil, status.Errorf(codes.InvalidArgument,
				"len(%x): %v, want %v", l.Index, got, want)
		}
		// TODO(gbelvin) use LeafHash rather than computing here.
		l.LeafHash = hasher.HashLeaf(l.LeafValue)

		if err = tx.Set(ctx, l.Index, *l); err != nil {
			return nil, err
		}
		if err = smtWriter.SetLeaves(ctx, []merkle.HashKeyValue{
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
	if err = tx.StoreSignedMapRoot(ctx, newRoot); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%v: Commit failed for SetLeaves: %v", mapID, err)
		return nil, err
	}

	return &trillian.SetMapLeavesResponse{
		MapRoot: &newRoot,
	}, nil
}

// GetSignedMapRoot implements the GetSignedMapRoot RPC method.
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (*trillian.GetSignedMapRootResponse, error) {
	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	r, err := tx.LatestSignedMapRoot(ctx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%v: Commit failed for GetSignedMapRoot: %v", req.MapId, err)
		return nil, err
	}

	return &trillian.GetSignedMapRootResponse{
		MapRoot: &r,
	}, nil
}

// GetSignedMapRootByRevision implements the GetSignedMapRootByRevision RPC
// method.
func (t *TrillianMapServer) GetSignedMapRootByRevision(ctx context.Context, req *trillian.GetSignedMapRootByRevisionRequest) (*trillian.GetSignedMapRootResponse, error) {
	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, req.MapId)
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	r, err := tx.GetSignedMapRoot(ctx, req.Revision)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%v: Commit failed for GetSignedMapRootByRevision: %v", req.MapId, err)
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

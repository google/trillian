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

package server

import (
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes/any"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// Init creates the initial revision 0 SignedMapHead, if one doesn't already exist.
func (t *TrillianMapServer) Init(ctx context.Context, mapID int64) error {
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, false /* readonly */)
	if err != nil {
		return err
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.registry.MapStorage.BeginForTree(ctx, mapID)
	if err != storage.ErrMapNeedsInit && err != nil {
		return err
	}
	defer tx.Close()

	if err == nil {
		// Init() not needed.
		return nil
	}

	glog.V(2).Infof("%v: Need to init map root revision 0", mapID)

	// TODO(phad): Refactor the SetLeaves func to avoid the duplication that follows.
	smtWriter, err := merkle.NewSparseMerkleTreeWriter(
		ctx,
		mapID,
		0, /* write revision */
		hasher, func() (storage.TreeTX, error) {
			ttx, err := t.registry.MapStorage.BeginForTree(ctx, mapID)
			if err == storage.ErrMapNeedsInit {
				err = nil // Init() is ongoing, so ignore this.
			}
			return ttx, err
		})
	if err != nil {
		return err
	}

	rootHash, err := smtWriter.CalculateRoot()
	if err != nil {
		return fmt.Errorf("CalculateRoot(): %v", err)
	}

	rev0Root, err := t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, mapID, 0 /*revision*/, nil /* metadata */)
	if err != nil {
		return fmt.Errorf("makeSignedMapRoot(): %v", err)
	}

	if err = tx.StoreSignedMapRoot(ctx, *rev0Root); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%v: Commit failed for SetLeaves: %v", mapID, err)
		return err
	}

	return nil
}

// GetLeaves implements the GetLeaves RPC method.  Each requested index will
// return an inclusion proof to either the leaf, or nil if the leaf does not
// exist.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (*trillian.GetMapLeavesResponse, error) {
	mapID := req.MapId
	if err := t.Init(ctx, mapID); err != nil {
		return nil, err
	}

	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, true /* readonly */)
	if err != nil {
		return nil, fmt.Errorf("could not get map %v: %v", mapID, err)
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("could not create database snapshot: %v", err)
	}
	defer tx.Close()

	var root *trillian.SignedMapRoot
	if req.Revision < 0 {
		// need to know the newest published revision
		r, err := tx.LatestSignedMapRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not fetch the latest SignedMapRoot: %v", err)
		}
		root = &r
	} else {
		r, err := tx.GetSignedMapRoot(ctx, req.Revision)
		if err != nil {
			return nil, fmt.Errorf("could not fetch SignedMapRoot %v: %v", req.Revision, err)
		}
		root = &r
	}

	smtReader := merkle.NewSparseMerkleTreeReader(root.MapRevision, hasher, tx)

	inclusions := make([]*trillian.MapLeafInclusion, 0, len(req.Index))
	found := 0
	for _, index := range req.Index {
		if got, want := len(index), hasher.Size(); got != want {
			return nil, status.Errorf(codes.InvalidArgument,
				"index len(%x): %v, want %v", index, got, want)
		}
		// Fetch the leaf if it exists.
		leaves, err := tx.Get(ctx, root.MapRevision, [][]byte{index})
		if err != nil {
			return nil, fmt.Errorf("could not fetch leaf %x: %v", index, err)
		}
		var leaf *trillian.MapLeaf
		if len(leaves) == 1 {
			leaf = &leaves[0]
			found++
		} else {
			// Empty leaf for proof of non-existence.
			leafHash, err := hasher.HashLeaf(mapID, index, nil)
			if err != nil {
				return nil, fmt.Errorf("HashLeaf(nil): %v", err)
			}
			leaf = &trillian.MapLeaf{
				Index:     index,
				LeafValue: nil,
				LeafHash:  leafHash,
			}
		}

		// Fetch the proof regardless of whether the leaf exists.
		proof, err := smtReader.InclusionProof(ctx, root.MapRevision, index)
		if err != nil {
			return nil, fmt.Errorf("could not get inclusion proof for leaf %x: %v", index, err)
		}

		inclusions = append(inclusions, &trillian.MapLeafInclusion{
			Leaf:      leaf,
			Inclusion: proof,
		})
	}
	glog.Infof("%v: wanted %v leaves, found %v", mapID, len(req.Index), found)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit db transaction: %v", err)
	}

	return &trillian.GetMapLeavesResponse{
		MapLeafInclusion: inclusions,
		MapRoot:          root,
	}, nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (*trillian.SetMapLeavesResponse, error) {
	mapID := req.MapId
	if err := t.Init(ctx, mapID); err != nil {
		return nil, err
	}

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
		req.MapId,
		tx.WriteRevision(),
		hasher, func() (storage.TreeTX, error) {
			return t.registry.MapStorage.BeginForTree(ctx, req.MapId)
		})
	if err != nil {
		return nil, err
	}

	for _, l := range req.Leaves {
		if got, want := len(l.Index), hasher.Size(); got != want {
			return nil, status.Errorf(codes.InvalidArgument,
				"len(%x): %v, want %v", l.Index, got, want)
		}
		if l.LeafValue == nil {
			// Leaves are empty by default. Do not allow clients to store
			// empty leaf values as this messes up the calculation of empty
			// branches.
			continue
		}
		// TODO(gbelvin) use LeafHash rather than computing here. #423
		leafHash, err := hasher.HashLeaf(mapID, l.Index, l.LeafValue)
		if err != nil {
			return nil, fmt.Errorf("HashLeaf(): %v", err)
		}
		l.LeafHash = leafHash

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
	if err != nil {
		return nil, fmt.Errorf("CalculateRoot(): %v", err)
	}

	newRoot, err := t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, req.MapId, tx.WriteRevision(), req.Metadata)
	if err != nil {
		return nil, fmt.Errorf("makeSignedMapRoot(): %v", err)
	}

	// TODO(al): need an smtWriter.Rollback() or similar I think.
	if err = tx.StoreSignedMapRoot(ctx, *newRoot); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("%v: Commit failed for SetLeaves: %v", mapID, err)
		return nil, err
	}

	return &trillian.SetMapLeavesResponse{MapRoot: newRoot}, nil
}

func (t *TrillianMapServer) makeSignedMapRoot(ctx context.Context, tree *trillian.Tree, smrTs time.Time,
	rootHash []byte, mapID, revision int64, meta *any.Any) (*trillian.SignedMapRoot, error) {
	smr := &trillian.SignedMapRoot{
		TimestampNanos: smrTs.UnixNano(),
		RootHash:       rootHash,
		MapId:          mapID,
		MapRevision:    revision,
		Metadata:       meta,
	}
	signer, err := trees.Signer(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("trees.Signer(): %v", err)
	}
	sig, err := signer.SignObject(smr)
	if err != nil {
		return nil, fmt.Errorf("SignObject(): %v", err)
	}
	smr.Signature = sig
	return smr, nil
}

// GetSignedMapRoot implements the GetSignedMapRoot RPC method.
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (*trillian.GetSignedMapRootResponse, error) {
	if err := t.Init(ctx, req.MapId); err != nil {
		return nil, err
	}
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
	if err := t.Init(ctx, req.MapId); err != nil {
		return nil, err
	}
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

func (t *TrillianMapServer) getTreeAndHasher(ctx context.Context, treeID int64, readonly bool) (*trillian.Tree, hashers.MapHasher, error) {
	tree, err := trees.GetTree(
		ctx,
		t.registry.AdminStorage,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: readonly})
	if err != nil {
		return nil, nil, err
	}
	th, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, err
	}
	return tree, th, nil
}

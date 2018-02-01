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

const (
	// Used internally by GetLeaves.
	mostRecentRevision = -1
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
	return t.getLeavesByRevision(ctx, req.MapId, req.Index, mostRecentRevision)
}

// GetLeavesByRevision implements the GetLeavesByRevision RPC method.
func (t *TrillianMapServer) GetLeavesByRevision(ctx context.Context, req *trillian.GetMapLeavesByRevisionRequest) (*trillian.GetMapLeavesResponse, error) {
	if req.Revision < 0 {
		return nil, fmt.Errorf("map revision %d must be >= 0", req.Revision)
	}
	return t.getLeavesByRevision(ctx, req.MapId, req.Index, req.Revision)
}

func (t *TrillianMapServer) getLeavesByRevision(ctx context.Context, mapID int64, indices [][]byte, revision int64) (*trillian.GetMapLeavesResponse, error) {
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
	if revision < 0 {
		// need to know the newest published revision
		r, err := tx.LatestSignedMapRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not fetch the latest SignedMapRoot: %v", err)
		}
		root = &r
	} else {
		r, err := tx.GetSignedMapRoot(ctx, revision)
		if err != nil {
			return nil, fmt.Errorf("could not fetch SignedMapRoot %v: %v", revision, err)
		}
		root = &r
	}

	smtReader := merkle.NewSparseMerkleTreeReader(root.MapRevision, hasher, tx)

	inclusions := make([]*trillian.MapLeafInclusion, 0, len(indices))
	found := 0
	for _, index := range indices {
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
	glog.Infof("%v: wanted %v leaves, found %v", mapID, len(indices), found)

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
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, false /* readonly */)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	var newRoot *trillian.SignedMapRoot
	err = t.registry.MapStorage.ReadWriteTransaction(ctx, req.MapId, func(ctx context.Context, tx storage.MapTreeTX) error {
		glog.V(2).Infof("%v: Writing at revision %v", mapID, tx.WriteRevision())
		smtWriter, err := merkle.NewSparseMerkleTreeWriter(
			ctx,
			req.MapId,
			tx.WriteRevision(),
			hasher, func(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
				return t.registry.MapStorage.ReadWriteTransaction(ctx, req.MapId, f)
			})
		if err != nil {
			return err
		}

		for _, l := range req.Leaves {
			if got, want := len(l.Index), hasher.Size(); got != want {
				return status.Errorf(codes.InvalidArgument,
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
				return fmt.Errorf("HashLeaf(): %v", err)
			}
			l.LeafHash = leafHash

			if err = tx.Set(ctx, l.Index, *l); err != nil {
				return err
			}
			if err = smtWriter.SetLeaves(ctx, []merkle.HashKeyValue{
				{
					HashedKey:   l.Index,
					HashedValue: l.LeafHash,
				},
			}); err != nil {
				return err
			}
		}

		rootHash, err := smtWriter.CalculateRoot()
		if err != nil {
			return fmt.Errorf("CalculateRoot(): %v", err)
		}

		newRoot, err = t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, req.MapId, tx.WriteRevision(), req.Metadata)
		if err != nil {
			return fmt.Errorf("makeSignedMapRoot(): %v", err)
		}

		// TODO(al): need an smtWriter.Rollback() or similar I think.
		if err = tx.StoreSignedMapRoot(ctx, *newRoot); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
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
	sig, err := signer.SignMapRoot(smr)
	if err != nil {
		return nil, fmt.Errorf("SignMapRoot(): %v", err)
	}
	smr.Signature = sig
	return smr, nil
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
	if req.Revision < 0 {
		return nil, fmt.Errorf("map revision %d must be >= 0", req.Revision)
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

// InitMap implements the RPC Method of the same name.
func (t *TrillianMapServer) InitMap(ctx context.Context, req *trillian.InitMapRequest) (*trillian.InitMapResponse, error) {
	mapID := req.MapId
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, false /* readonly */)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "getTreeAndHasher(): %v", err)
	}
	ctx = trees.NewContext(ctx, tree)

	var rev0Root *trillian.SignedMapRoot
	err = t.registry.MapStorage.ReadWriteTransaction(ctx, mapID, func(ctx context.Context, tx storage.MapTreeTX) error {
		if smr, err := tx.LatestSignedMapRoot(ctx); err == nil && smr.TimestampNanos != 0 {
			// No need to init - we already have a SignedMapRoot.
			return status.New(codes.AlreadyExists, "map already intialised.").Err()
		}

		rev0Root = nil
		defer tx.Close()

		// Check that the map actually needs initialising
		latestRoot, err := tx.LatestSignedMapRoot(ctx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			return nil, status.Errorf(codes.FailedPrecondition, "LatestSignedMapRoot(): %v", err)
		}
		// Belt and braces check.
		if latestRoot.GetRootHash() != nil {
			return nil, status.Errorf(codes.AlreadyExists, "map is already initialised")
		}

		glog.V(2).Infof("%v: Need to init map root revision 0", mapID)
		rootHash := hasher.HashEmpty(mapID, make([]byte, hasher.Size()), hasher.BitLen())
		var err error
		rev0Root, err = t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, mapID, 0 /*revision*/, nil /* metadata */)
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
	})
	if err != nil {
		return nil, err
	}

	return &trillian.InitMapResponse{
		Created: rev0Root,
	}, nil
}

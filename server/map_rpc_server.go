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
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Used internally by GetLeaves.
	mostRecentRevision = -1
)

var (
	optsMapInit  = trees.NewGetOpts(trees.Admin, trillian.TreeType_MAP)
	optsMapRead  = trees.NewGetOpts(trees.Query, trillian.TreeType_MAP)
	optsMapWrite = trees.NewGetOpts(trees.UpdateMap, trillian.TreeType_MAP)
)

// TODO(codingllama): There is no access control in the server yet and clients could easily modify
// any tree.

// TrillianMapServerOptions allows various options to be provided when creating
// a new TrillianMapServer.
type TrillianMapServerOptions struct {
	// UseSingleTransaction specifies whether updates to a map should be
	// attempted within a single transaction.
	UseSingleTransaction bool
}

// TrillianMapServer implements the RPC API defined in the proto
type TrillianMapServer struct {
	registry extension.Registry
	opts     TrillianMapServerOptions
}

// NewTrillianMapServer creates a new RPC server backed by registry
func NewTrillianMapServer(registry extension.Registry, opts TrillianMapServerOptions) *TrillianMapServer {
	if opts.UseSingleTransaction {
		glog.Warning("Using experimental single-transaction mode for map server.")
	}
	return &TrillianMapServer{registry: registry, opts: opts}
}

// IsHealthy returns nil if the server is healthy, error otherwise.
func (t *TrillianMapServer) IsHealthy() error {
	ctx, span := spanFor(context.Background(), "IsHealthy")
	defer span.End()
	return t.registry.MapStorage.CheckDatabaseAccessible(ctx)
}

// GetLeaves implements the GetLeaves RPC method.  Each requested index will
// return an inclusion proof to either the leaf, or nil if the leaf does not
// exist.
func (t *TrillianMapServer) GetLeaves(ctx context.Context, req *trillian.GetMapLeavesRequest) (*trillian.GetMapLeavesResponse, error) {
	ctx, span := spanFor(ctx, "GetLeaves")
	defer span.End()
	return t.getLeavesByRevision(ctx, req.MapId, req.Index, mostRecentRevision)
}

// GetLeavesByRevision implements the GetLeavesByRevision RPC method.
func (t *TrillianMapServer) GetLeavesByRevision(ctx context.Context, req *trillian.GetMapLeavesByRevisionRequest) (*trillian.GetMapLeavesResponse, error) {
	ctx, span := spanFor(ctx, "GetLeavesByRevision")
	defer span.End()
	if req.Revision < 0 {
		return nil, fmt.Errorf("map revision %d must be >= 0", req.Revision)
	}
	return t.getLeavesByRevision(ctx, req.MapId, req.Index, req.Revision)
}

func (t *TrillianMapServer) getLeavesByRevision(ctx context.Context, mapID int64, indices [][]byte, revision int64) (*trillian.GetMapLeavesResponse, error) {
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, optsMapRead)
	if err != nil {
		return nil, fmt.Errorf("could not get map %v: %v", mapID, err)
	}
	ctx = trees.NewContext(ctx, tree)

	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, tree)
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

	var mapRoot types.MapRootV1
	if err := mapRoot.UnmarshalBinary(root.MapRoot); err != nil {
		return nil, err
	}

	smtReader := merkle.NewSparseMerkleTreeReader(int64(mapRoot.Revision), hasher, tx)

	inclusions := make([]*trillian.MapLeafInclusion, 0, len(indices))
	found := 0
	for _, index := range indices {
		if err := checkIndexSize(index, hasher); err != nil {
			return nil, err
		}
		// Fetch the leaf if it exists.
		leaves, err := tx.Get(ctx, int64(mapRoot.Revision), [][]byte{index})
		if err != nil {
			return nil, fmt.Errorf("could not fetch leaf %x: %v", index, err)
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
				LeafHash:  nil, // equivalent to hasher.HashEmpty(mapID, index, 0)
			}
		}

		// Fetch the proof regardless of whether the leaf exists.
		proof, err := smtReader.InclusionProof(ctx, int64(mapRoot.Revision), index)
		if err != nil {
			return nil, fmt.Errorf("could not get inclusion proof for leaf %x: %v", index, err)
		}

		inclusions = append(inclusions, &trillian.MapLeafInclusion{
			Leaf:      leaf,
			Inclusion: proof,
		})
	}
	glog.V(1).Infof("%v: wanted %v leaves, found %v", mapID, len(indices), found)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit db transaction: %v", err)
	}

	return &trillian.GetMapLeavesResponse{
		MapLeafInclusion: inclusions,
		MapRoot:          root,
	}, nil
}

func checkIndexSize(index []byte, hasher hashers.MapHasher) error {
	// The parameter is named 'index' (here and in the RPC API) because it's the ordinal number
	// of the leaf, but that number is obtained by hashing the key value that corresponds to the
	// leaf.  Leaf "indices" are therefore sparsely scattered in the range [0, 2^hashsize) and
	// are represented as a []byte, and every leaf must have an index that is the same size.
	//
	// We currently police this by requiring that the hash size for the index space be the same
	// as the hash size for the tree itself, although that's not strictly required (e.g. could
	// have SHA-256 for generating leaf indices, but SHA-512 for building the root hash).
	if len(index) != hasher.Size() {
		return status.Errorf(codes.InvalidArgument, "index len(%x) is not %d", index, hasher.Size())
	}
	return nil
}

// SetLeaves implements the SetLeaves RPC method.
func (t *TrillianMapServer) SetLeaves(ctx context.Context, req *trillian.SetMapLeavesRequest) (*trillian.SetMapLeavesResponse, error) {
	// Check leaves all have unique indices.
	seen := make(map[string]bool)
	for _, leaf := range req.Leaves {
		k := fmt.Sprintf("%x", leaf.Index)
		if seen[k] {
			return nil, status.Errorf(codes.InvalidArgument, "index %s duplicated", k)
		}
		seen[k] = true
	}

	ctx, span := spanFor(ctx, "SetLeaves")
	defer span.End()
	mapID := req.MapId
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, optsMapWrite)
	if err != nil {
		return nil, err
	}
	ctx = trees.NewContext(ctx, tree)

	txRolledUp := uint64(0)

	var newRoot *trillian.SignedMapRoot
	err = t.registry.MapStorage.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		writeRev, err := tx.WriteRevision(ctx)
		if err != nil {
			return err
		}
		glog.V(2).Infof("%v: Writing at revision %v", mapID, writeRev)
		smtWriter, err := merkle.NewSparseMerkleTreeWriter(
			ctx,
			req.MapId,
			writeRev,
			hasher, func(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
				if t.opts.UseSingleTransaction {
					glog.V(1).Infof("Using enclosing tx for subtree operation %d", atomic.LoadUint64(&txRolledUp))
					atomic.AddUint64(&txRolledUp, 1)
					return f(ctx, tx)
				}
				return t.registry.MapStorage.ReadWriteTransaction(ctx, tree, f)
			})
		if err != nil {
			return err
		}

		for _, l := range req.Leaves {
			if err := checkIndexSize(l.Index, hasher); err != nil {
				return err
			}
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

		if t.opts.UseSingleTransaction {
			glog.V(1).Infof("Rolled %d transactions up into single commit", atomic.LoadUint64(&txRolledUp))
		}

		rootHash, err := smtWriter.CalculateRoot()
		if err != nil {
			return fmt.Errorf("CalculateRoot(): %v", err)
		}

		newRoot, err = t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, req.MapId, writeRev, req.Metadata)
		if err != nil {
			return fmt.Errorf("makeSignedMapRoot(): %v", err)
		}

		// TODO(al): need an smtWriter.Rollback() or similar I think.
		return tx.StoreSignedMapRoot(ctx, *newRoot)
	})
	if err != nil {
		return nil, err
	}
	return &trillian.SetMapLeavesResponse{MapRoot: newRoot}, nil
}

func (t *TrillianMapServer) makeSignedMapRoot(ctx context.Context, tree *trillian.Tree, smrTs time.Time,
	rootHash []byte, mapID, revision int64, meta []byte) (*trillian.SignedMapRoot, error) {
	smr := &types.MapRootV1{
		RootHash:       rootHash,
		TimestampNanos: uint64(smrTs.UnixNano()),
		Revision:       uint64(revision),
		Metadata:       meta,
	}
	signer, err := trees.Signer(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("trees.Signer(): %v", err)
	}
	root, err := signer.SignMapRoot(smr)
	if err != nil {
		return nil, fmt.Errorf("SignMapRoot(): %v", err)
	}
	return root, nil
}

// GetSignedMapRoot implements the GetSignedMapRoot RPC method.
func (t *TrillianMapServer) GetSignedMapRoot(ctx context.Context, req *trillian.GetSignedMapRootRequest) (*trillian.GetSignedMapRootResponse, error) {
	ctx, span := spanFor(ctx, "GetSignedMapRoot")
	defer span.End()
	tree, ctx, err := t.getTreeAndContext(ctx, req.MapId, optsMapRead)
	if err != nil {
		return nil, err
	}
	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, tree)
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
	ctx, span := spanFor(ctx, "GetSignedMapRootByRevision")
	defer span.End()
	if req.Revision < 0 {
		return nil, fmt.Errorf("map revision %d must be >= 0", req.Revision)
	}
	tree, ctx, err := t.getTreeAndContext(ctx, req.MapId, optsMapRead)
	if err != nil {
		return nil, err
	}
	tx, err := t.registry.MapStorage.SnapshotForTree(ctx, tree)
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

func (t *TrillianMapServer) getTreeAndHasher(ctx context.Context, treeID int64, opts trees.GetOpts) (*trillian.Tree, hashers.MapHasher, error) {
	tree, err := trees.GetTree(ctx, t.registry.AdminStorage, treeID, opts)
	if err != nil {
		return nil, nil, err
	}
	th, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, err
	}
	return tree, th, nil
}

func (t *TrillianMapServer) getTreeAndContext(ctx context.Context, treeID int64, opts trees.GetOpts) (*trillian.Tree, context.Context, error) {
	tree, err := trees.GetTree(ctx, t.registry.AdminStorage, treeID, opts)
	if err != nil {
		return nil, nil, err
	}
	return tree, trees.NewContext(ctx, tree), nil
}

// InitMap implements the RPC Method of the same name.
func (t *TrillianMapServer) InitMap(ctx context.Context, req *trillian.InitMapRequest) (*trillian.InitMapResponse, error) {
	ctx, span := spanFor(ctx, "InitMap")
	defer span.End()
	mapID := req.MapId
	tree, hasher, err := t.getTreeAndHasher(ctx, mapID, optsMapInit)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "getTreeAndHasher(): %v", err)
	}
	ctx = trees.NewContext(ctx, tree)

	var rev0Root *trillian.SignedMapRoot
	err = t.registry.MapStorage.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		// Check that the map actually needs initialising
		latestRoot, err := tx.LatestSignedMapRoot(ctx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			return status.Errorf(codes.FailedPrecondition, "LatestSignedMapRoot(): %v", err)
		}
		// Belt and braces check.
		if latestRoot.GetMapRoot() != nil {
			return status.Errorf(codes.AlreadyExists, "map is already initialised")
		}

		rev0Root = nil

		glog.V(2).Infof("%v: Need to init map root revision 0", mapID)
		rootHash := hasher.HashEmpty(mapID, make([]byte, hasher.Size()), hasher.BitLen())
		rev0Root, err = t.makeSignedMapRoot(ctx, tree, time.Now(), rootHash, mapID, 0 /*revision*/, nil /* metadata */)
		if err != nil {
			return fmt.Errorf("makeSignedMapRoot(): %v", err)
		}

		return tx.StoreSignedMapRoot(ctx, *rev0Root)
	})
	if err != nil {
		return nil, err
	}

	return &trillian.InitMapResponse{
		Created: rev0Root,
	}, nil
}

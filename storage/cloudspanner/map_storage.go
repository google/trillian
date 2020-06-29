// Copyright 2018 Google LLC. All Rights Reserved.
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

package cloudspanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/storagepb/convert"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	mapLeafDataTbl = "MapLeafData"
	// Spanner DB columns:
	colLeafIndex   = "LeafIndex"
	colMapRevision = "MapRevision"
	colLeafHash    = "LeafHash"
)

var errFinished = errors.New("finished")

// MapStorageOptions is used to configure various parameters of the spanner map storage layer.
type MapStorageOptions struct {
	TreeStorageOptions
}

// NewMapStorage initialises and returns a new MapStorage.
func NewMapStorage(_ context.Context, client *spanner.Client) storage.MapStorage {
	return NewMapStorageWithOpts(client, MapStorageOptions{})
}

// NewMapStorageWithOpts initialises and returns a new MapStorage using options.
func NewMapStorageWithOpts(client *spanner.Client, opts MapStorageOptions) storage.MapStorage {
	ret := &mapStorage{
		ts:   newTreeStorageWithOpts(client, opts.TreeStorageOptions),
		opts: opts,
	}

	return ret
}

// mapStorage is a CloudSpanner backed trillian.MapStorage implementation.
type mapStorage struct {
	// ts provides the merkle-tree level primitives which are built upon by this
	// mapStorage.
	ts *treeStorage

	opts MapStorageOptions
}

func (ms *mapStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, ms.ts.client)
}

func newMapCache(tree *trillian.Tree) (*cache.SubtreeCache, error) {
	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}
	return cache.NewMapSubtreeCache(defMapStrata, tree.TreeId, hasher), nil
}

// Returns a ready-to-use MapTreeTX.
func (ms *mapStorage) begin(ctx context.Context, tree *trillian.Tree, readonly bool, stx spanRead) (*mapTX, error) {
	tx, err := ms.ts.begin(ctx, tree, newMapCache, stx)
	if err != nil {
		glog.Errorf("failed to treeStorage.begin(treeID=%d): %v", tree.TreeId, err)
		return nil, err
	}

	return ms.createMapTX(tx)
}

func (ms *mapStorage) createMapTX(tx *treeTX) (*mapTX, error) {
	// Sanity check tx.config
	if cfg, ok := tx.config.(*spannerpb.MapStorageConfig); !ok || cfg == nil {
		return nil, fmt.Errorf("unexpected config type for MAP tree %v: %T", tx.treeID, tx.config)
	}

	return &mapTX{
		treeTX: tx,
		ms:     ms,
	}, nil
}

func (ms *mapStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyMapTreeTX, error) {
	return ms.begin(ctx, tree, true, ms.ts.client.ReadOnlyTransaction())
}

// Layout returns the layout of the given tree.
func (ms *mapStorage) Layout(tree *trillian.Tree) (*tree.Layout, error) {
	return defaultMapLayout, nil
}

func (ms *mapStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.MapTXFunc) error {
	_, err := ms.ts.client.ReadWriteTransaction(ctx, func(ctx context.Context, stx *spanner.ReadWriteTransaction) error {
		tx, err := ms.begin(ctx, tree, false /* readonly */, stx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			glog.Errorf("failed to mapStorage.begin(treeID=%d): %v", tree.TreeId, err)
			return err
		}
		if err := f(ctx, tx); err != nil {
			return err
		}
		if err := tx.flushSubtrees(ctx); err != nil {
			glog.Errorf("failed to tx.flushSubtrees(): %v", err)
			return err
		}
		return nil
	})
	return err
}

// mapTX is a concrete implementation of the Trillian storage.MapStorage
// interface.
type mapTX struct {
	// treeTX embeds the merkle-tree level transactional actions.
	*treeTX
	// ms is the MapStorage which begat this mapTX.
	ms *mapStorage
}

// sthToSMR converts a spannerpb.TreeHead to a trillian.SignedMapRoot.
func sthToSMR(sth *spannerpb.TreeHead) (*trillian.SignedMapRoot, error) {
	mapRoot, err := (&types.MapRootV1{
		RootHash:       sth.RootHash,
		TimestampNanos: uint64(sth.TsNanos),
		Revision:       uint64(sth.TreeRevision),
		Metadata:       sth.Metadata,
	}).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &trillian.SignedMapRoot{
		MapRoot:   mapRoot,
		Signature: sth.Signature,
	}, nil
}

// LatestSignedMapRoot returns the freshest SignedMapRoot for this map at the
// time the transaction was started.
func (tx *mapTX) LatestSignedMapRoot(ctx context.Context) (*trillian.SignedMapRoot, error) {
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		glog.Errorf("failed to determine current STH: %v", err)
		return nil, err
	}
	writeRev, err := tx.writeRev(ctx)
	if err != nil {
		glog.Errorf("failed to determine write revision: %v", err)
		return nil, err
	}
	if got, want := currentSTH.TreeRevision+1, writeRev; got != want {
		return nil, fmt.Errorf("inconsistency: currentSTH.TreeRevision+1 (%d) != writeRev (%d)", got, want)
	}

	// We already read the latest root as part of starting the transaction (in
	// order to calculate the writeRevision), so we just return that data here:
	return sthToSMR(currentSTH)
}

// StoreSignedMapRoot stores the provided root.
// This method will return an error if the caller attempts to store more than
// one root per map for a given map revision.
func (tx *mapTX) StoreSignedMapRoot(ctx context.Context, root *trillian.SignedMapRoot) error {
	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}

	writeRev, err := tx.writeRev(ctx)
	if err != nil {
		glog.Errorf("failed to determine write revision: %v", err)
		return err
	}

	var r types.MapRootV1
	if err := r.UnmarshalBinary(root.MapRoot); err != nil {
		return err
	}
	sth := spannerpb.TreeHead{
		TsNanos:      int64(r.TimestampNanos),
		RootHash:     r.RootHash,
		Metadata:     r.Metadata,
		TreeId:       tx.treeID,
		TreeRevision: writeRev,
		Signature:    root.Signature,
	}

	// TODO(al): consider replacing these with InsertStruct throughout.
	// TODO(al): consider making TreeSize nullable.
	m := spanner.Insert(
		treeHeadTbl,
		[]string{
			"TreeID",
			"TimestampNanos",
			"TreeSize",
			"RootHash",
			"RootSignature",
			"TreeRevision",
			"TreeMetadata",
		},
		[]interface{}{
			int64(tx.treeID),
			int64(sth.TsNanos),
			0,
			sth.RootHash,
			sth.Signature,
			writeRev,
			sth.Metadata,
		})

	return stx.BufferWrite([]*spanner.Mutation{m})
}

// Set sets the leaf with the specified index to value.
// Returns an error if there's a problem with the underlying storage.
func (tx *mapTX) Set(ctx context.Context, index []byte, value *trillian.MapLeaf) error {
	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	writeRev, err := tx.writeRev(ctx)
	if err != nil {
		glog.Errorf("failed to determine write revision: %v", err)
		return err
	}

	if !bytes.Equal(index, value.Index) {
		return fmt.Errorf("map_storage inconsistency: index (%x) != value.LeafIndex (%x)", index, value.Index)
	}

	// A nil value needs to be translated to an empty slice as the LeafValue column is 'NOT NULL'.
	leafValue := value.LeafValue
	if leafValue == nil {
		leafValue = []byte{}
	}

	m := spanner.Insert(mapLeafDataTbl,
		[]string{colTreeID, colLeafIndex, colMapRevision, colLeafHash, colLeafValue, colExtraData},
		[]interface{}{tx.treeID, index, writeRev, value.LeafHash, leafValue, value.ExtraData})

	return stx.BufferWrite([]*spanner.Mutation{m})
}

// GetTiles reads the Merkle tree tiles with the given root IDs at the given
// revision. A tile is empty if it is missing from the returned slice.
func (tx *mapTX) GetTiles(ctx context.Context, rev int64, ids []tree.NodeID2) ([]smt.Tile, error) {
	rootIDs := make([]tree.NodeID, 0, len(ids))
	for _, id := range ids {
		rootIDs = append(rootIDs, tree.NewNodeIDFromID2(id))
	}

	getTilesFn, err := tx.treeTX.getTilesFunc(ctx, rev)
	if err != nil {
		return nil, err
	}
	subtrees, err := getTilesFn(rootIDs)
	if err != nil {
		return nil, err
	}

	tiles := make([]smt.Tile, 0, len(subtrees))
	for _, subtree := range subtrees {
		tile, err := convert.Unmarshal(subtree)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, tile)
	}
	return tiles, nil
}

func (t *treeTX) getTilesFunc(ctx context.Context, rev int64) (func([]storage.NodeID) ([]*storagepb.SubtreeProto, error), error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stx == nil {
		return nil, ErrTransactionClosed
	}
	return t.parallelGetMerkleNodes(ctx, rev), nil
}

func (t *treeTX) parallelGetMerkleNodes(ctx context.Context, rev int64) func([]storage.NodeID) ([]*storagepb.SubtreeProto, error) {
	return func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
		ctx, span := trace.StartSpan(ctx, "TreeTX.parallelGetMerkleNodes")
		defer span.End()
		// Request the various subtrees in parallel.
		// Creates one goroutine per tile.

		// c will carry any retrieved subtrees
		c := make(chan *storagepb.SubtreeProto, len(ids))
		g, gctx := errgroup.WithContext(ctx)
		for _, id := range ids {
			id := id
			g.Go(func() error {
				st, err := t.getSubtree(gctx, rev, id)
				if err != nil {
					return fmt.Errorf("failed to treeTX.getSubtree(rev=%d, id=%x): %v", rev, id, err)
				}
				c <- st
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
		close(c)
		ret := make([]*storagepb.SubtreeProto, 0, len(ids))
		for st := range c {
			if st != nil {
				ret = append(ret, st)
			}
		}
		return ret, nil
	}
}

// SetTiles stores the given tiles at the current write revision.
func (tx *mapTX) SetTiles(ctx context.Context, tiles []smt.Tile) error {
	subs := make([]*storagepb.SubtreeProto, 0, len(tiles))
	for _, tile := range tiles {
		height := defaultMapLayout.TileHeight(int(tile.ID.BitLen()))
		pb, err := convert.Marshal(tile, uint(height))
		if err != nil {
			return err
		}
		subs = append(subs, pb)
	}
	return tx.storeSubtrees(ctx, subs)
}

// getMapLeaf fetches and returns the MapLeaf stored at the specified index and
// revision.
func (tx *mapTX) getMapLeaf(ctx context.Context, revision int64, index []byte) (*trillian.MapLeaf, error) {
	cols := []string{colLeafIndex, colMapRevision, colLeafHash, colLeafValue, colExtraData}
	rowKey := spanner.Key{tx.treeID, index}.AsPrefix()
	var l *trillian.MapLeaf
	rows := tx.stx.Read(ctx, mapLeafDataTbl, spanner.KeySets(rowKey), cols)
	err := rows.Do(func(r *spanner.Row) error {
		// TODO(alcutter): add MapRevision to trillian.MapLeaf
		var rev int64
		var leaf trillian.MapLeaf
		if err := r.Columns(&leaf.Index, &rev, &leaf.LeafHash, &leaf.LeafValue, &leaf.ExtraData); err != nil {
			return err
		}
		// Leaves are stored by descending revision, so the first one we find which
		// satisfies this condition is good:
		if rev <= revision {
			l = &leaf
			return errFinished
		}
		return nil
	})
	if err != nil && err != errFinished {
		glog.Errorf("failed to read MapLeafData row for rev %d index %x: %v", revision, index, err)
		return nil, err
	}
	return l, nil
}

// Get returns the values associated with indexes, at revision.
// Returns a slice of MapLeaf structs containing the requested values.
// The entries in the returned slice are not guaranteed to be in the same order
// as the corresponding values in indexes.
// An error will be returned if there is a problem with the underlying
// storage.
func (tx *mapTX) Get(ctx context.Context, revision int64, indexes [][]byte) ([]*trillian.MapLeaf, error) {
	// c will carry any retrieved MapLeaves.
	c := make(chan *trillian.MapLeaf, len(indexes))
	// errc will carry any errors while reading from spanner, although we'll only
	// return to the caller the first, if any.
	errc := make(chan error, len(indexes))

	for _, idx := range indexes {
		idx := idx
		go func() {
			l, err := tx.getMapLeaf(ctx, revision, idx)
			if err != nil {
				glog.Errorf("failed to getMapLeafData(rev=%d, index=%x): %v", revision, idx, err)
				errc <- err
				return
			}
			c <- l
		}()
	}

	// Now wait for the goroutines to do their thing.
	ret := make([]*trillian.MapLeaf, 0, len(indexes))
	for range indexes {
		select {
		case err := <-errc:
			return nil, err
		case l := <-c:
			if l != nil {
				ret = append(ret, l)
			}
		}
	}
	return ret, nil
}

// GetSignedMapRoot returns the SignedMapRoot for revision.
// An error will be returned if there is a problem with the underlying storage.
func (tx *mapTX) GetSignedMapRoot(ctx context.Context, revision int64) (*trillian.SignedMapRoot, error) {
	query := spanner.NewStatement(
		`SELECT t.TreeID, t.TimestampNanos, t.TreeSize, t.RootHash, t.RootSignature, t.TreeRevision, t.TreeMetadata FROM TreeHeads t
				WHERE t.TreeID = @tree_id
				AND t.TreeRevision = @tree_rev
				LIMIT 1`)
	query.Params["tree_id"] = tx.treeID
	query.Params["tree_rev"] = revision

	var th *spannerpb.TreeHead
	rows := tx.stx.Query(ctx, query)
	err := rows.Do(func(r *spanner.Row) error {
		tth := &spannerpb.TreeHead{}
		if err := r.Columns(&tth.TreeId, &tth.TsNanos, &tth.TreeSize, &tth.RootHash, &tth.Signature, &tth.TreeRevision, &tth.Metadata); err != nil {
			return err
		}

		th = tth
		return nil
	})
	if err != nil {
		return nil, err
	}
	if th == nil {
		if revision == 0 {
			return nil, storage.ErrTreeNeedsInit
		}
		return nil, status.Errorf(codes.NotFound, "map root %v not found", revision)
	}
	return sthToSMR(th)
}

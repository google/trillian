// Copyright 2019 Google Inc. All Rights Reserved.
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
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
	"golang.org/x/sync/errgroup"
)

// mapTreeUpdater updates the sparse Merkle tree of the map in one or multiple
// map tree transactions.
type mapTreeUpdater struct {
	tree     *trillian.Tree
	layout   *tree.Layout
	hasher   hashers.MapHasher
	ms       storage.MapStorage
	writeRev int64
	singleTX bool
	preload  bool
}

// update updates the sparse Merkle tree at the current write revision with the
// given leaf node updates, and writes it to the storage. Returns the new root
// hash. Requires nodes slice to be non-empty.
func (t *mapTreeUpdater) update(ctx context.Context, tx storage.MapTreeTX, nodes []smt.Node) ([]byte, error) {
	// Work around a performance issue when using the map in single-transaction
	// mode by preloading all the tiles we know the Writer is going to need.
	var preloaded *smt.TileSet
	if t.singleTX && t.preload {
		var err error
		if preloaded, err = t.doPreload(ctx, tx, nodes); err != nil {
			return nil, err
		}
	}

	// TODO(pavelkalinnikov): Make the layout configurable.
	const topHeight = uint(8) // The height of the top shard.
	w := smt.NewWriter(t.tree.TreeId, t.hasher, uint(t.hasher.BitLen()), topHeight)
	shards, err := w.Split(nodes) // Split the nodes into shards below topHeight.
	if err != nil {
		return nil, err
	}

	runTX := t.newTXFunc(tx)
	// The update function runs a read-write transaction that updates a shard of
	// the map tree: either one of the "leaf" shards, or the top shard.
	update := func(ctx context.Context, nodes []smt.Node) (root smt.Node, err error) {
		err = runTX(ctx, func(ctx context.Context, tx storage.MapTreeTX) error {
			nodesCopy := make([]smt.Node, len(nodes))
			copy(nodesCopy, nodes) // Protect from TX restarts.
			acc := &txAccessor{updater: t, tiles: preloaded, tx: tx}
			var err error
			root, err = w.Write(ctx, nodesCopy, acc)
			return err
		})
		return root, err
	}

	// shardRoots accumulates root updates for all the "leaf" shards, which is
	// then fed as an input to the topmost shard update.
	shardRoots := make([]smt.Node, 0, 1<<topHeight)
	var mu sync.Mutex // Guards topUpds.

	// Run update calculations for "leaf" shards in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	for _, nodes := range shards {
		nodes := nodes
		g.Go(func() error {
			shardRoot, err := update(gCtx, nodes)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			shardRoots = append(shardRoots, shardRoot)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// Note: There is a memory barrier in g.Wait() sufficient to not lock the
	// mutex guarding topUpds below this point.

	// Finally, update the topmost shard using the "leaf" shard roots updates.
	root, err := update(ctx, shardRoots)
	if err != nil {
		return nil, err
	}
	return root.Hash, nil
}

type txAccessor struct {
	updater *mapTreeUpdater
	// tiles is a cache of tree tiles that Get uses to fetch node hashes from,
	// and Set mutates tiles from. It is either specified when this accessor is
	// created (i.e. the hashes were preloaded), or otherwise set when the first
	// (and the only) Get call occurs. A missing tile is interpreted as empty.
	tiles *smt.TileSet
	tx    storage.MapTreeTX
}

// TODO(pavelkalinnikov): Get should take a list of tile IDs.
func (t *txAccessor) Get(ctx context.Context, ids []tree.NodeID2) (map[tree.NodeID2][]byte, error) {
	// TODO(pavelkalinnikov): Factor out preload into another accessor.
	if t.tiles == nil {
		var err error
		if t.tiles, err = t.updater.load(ctx, t.tx, ids); err != nil {
			return nil, err
		}
	}
	return t.tiles.Hashes(), nil
}

// Set applies the updates of the given nodes to the transaction. It accepts
// all nodes updated in the tree. However, only "leaf" nodes of tiles are used
// to build the updated tiles, before they are passed in to the storage layer.
func (t *txAccessor) Set(ctx context.Context, nodes []smt.Node) error {
	m := smt.NewTileSetMutation(t.tiles)
	for _, n := range nodes {
		m.Set(n.ID, n.Hash)
	}
	tiles, err := m.Build()
	if err != nil {
		return err
	}
	return t.tx.SetTiles(ctx, tiles)
}

type txFunc func(context.Context, func(context.Context, storage.MapTreeTX) error) error

func (t *mapTreeUpdater) newTXFunc(tx storage.MapTreeTX) txFunc {
	if t.singleTX {
		// Execute all calls with the same underlying transaction. If the function
		// is large, this may incur a performance penalty.
		return func(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
			return f(ctx, tx)
		}
	}
	// Execute each call in its own transaction. This allows each invocation of f
	// to proceed independently much faster. However, If one transaction fails,
	// the other can still succeed. In some cases this can cause data corruption.
	return func(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
		return t.ms.ReadWriteTransaction(ctx, t.tree, f)
	}
}

// doPreload loads all Merkle tree tiles that will be involved when updating
// the given set of nodes at the specified revision.
func (t *mapTreeUpdater) doPreload(ctx context.Context, tx storage.MapTreeTX, nodes []smt.Node) (*smt.TileSet, error) {
	ctx, spanEnd := spanFor(ctx, "doPreload")
	defer spanEnd()
	// TODO(pavelkalinnikov): Avoid using HStar3 directly.
	// TODO(pavelkalinnikov): Introduce layout "height" method and use it here.
	hs, err := smt.NewHStar3(nodes, nil, uint(t.hasher.BitLen()), 0)
	if err != nil {
		return nil, err
	}
	return t.load(ctx, tx, hs.Prepare())
}

// load fetches Merkle tree tiles containing the given node IDs.
func (t *mapTreeUpdater) load(ctx context.Context, tx storage.MapTreeTX, ids []tree.NodeID2) (*smt.TileSet, error) {
	tileIDs := toTileIDs(ids, t.layout)
	tiles, err := tx.GetTiles(ctx, t.writeRev-1, tileIDs)
	if err != nil {
		return nil, err
	}
	ts := smt.NewTileSet(t.tree.TreeId, t.hasher, t.layout)
	for _, tile := range tiles {
		// TODO(pavelkalinnikov): Check tile against the layout.
		if err := ts.Add(tile); err != nil {
			return nil, err
		}
	}
	return ts, nil
}

// toTileIDs returns the list of tile IDs that the given nodes belong to, in
// accordance with the layout.
func toTileIDs(ids []tree.NodeID2, layout *tree.Layout) []tree.NodeID2 {
	// Note: The capacity estimate assumes that most of the tile heights are 8.
	// It's not a strong requirement, as the map grows when necessary. The real
	// size also depends on IDs, so can differ even if all tile heights are 8.
	roots := make(map[tree.NodeID2]bool, len(ids)/8)
	for _, id := range ids {
		roots[layout.GetTileRootID(id)] = true
	}
	tileIDs := make([]tree.NodeID2, 0, len(roots))
	for id := range roots {
		tileIDs = append(tileIDs, id)
	}
	return tileIDs
}

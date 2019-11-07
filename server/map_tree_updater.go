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
	hasher   hashers.MapHasher
	ms       storage.MapStorage
	singleTX bool
	preload  bool
}

// update updates the sparse Merkle tree at the passed-in revision with the
// given leaf updates, and writes it to the storage. Returns the new root hash.
// Requires updates to be non-empty.
func (t *mapTreeUpdater) update(ctx context.Context, tx storage.MapTreeTX, upd []smt.NodeUpdate, writeRev int64) ([]byte, error) {
	// Work around a performance issue when using the map in single-transaction
	// mode by preloading all the nodes we know the Writers are going to need.
	var hashes map[tree.NodeID2][]byte
	preload := t.singleTX && t.preload
	// Note: It's fine if hashes == nil, it only happens if preload == false.
	if preload {
		var err error
		if hashes, err = doPreload(ctx, tx, uint(t.hasher.BitLen()), upd, writeRev-1); err != nil {
			return nil, err
		}
	}

	// TODO(pavelkalinnikov): Make the layout configurable.
	const topHeight = uint(8) // The height of the top shard.
	w := smt.NewWriter(t.tree.TreeId, t.hasher, uint(t.hasher.BitLen()), topHeight)
	shards, err := w.Split(upd) // Split the updates into shards below topHeight.
	if err != nil {
		return nil, err
	}

	runTX := t.newTXFunc(tx)
	// The update function runs a read-write transaction that updates a shard of
	// the map tree: either one of the "leaf" shards, or the top shard.
	update := func(ctx context.Context, upd []smt.NodeUpdate) (root smt.NodeUpdate, err error) {
		err = runTX(ctx, func(ctx context.Context, tx storage.MapTreeTX) error {
			updCopy := make([]smt.NodeUpdate, len(upd))
			copy(updCopy, upd) // Protect from TX restarts.
			acc := &txAccessor{hashes: hashes, preload: preload, tx: tx, rev: writeRev}
			var err error
			root, err = w.Write(ctx, updCopy, acc)
			return err
		})
		return root, err
	}

	// topUpds accumulates root updates for all the "leaf" shards, which is then
	// fed as an input to the topmost shard update.
	topUpds := make([]smt.NodeUpdate, 0, 1<<topHeight)
	var mu sync.Mutex // Guards topUpds.

	// Run update calculations for "leaf" shards in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	for _, upd := range shards {
		upd := upd
		g.Go(func() error {
			shardRootUpd, err := update(gCtx, upd)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			topUpds = append(topUpds, shardRootUpd)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// Note: There is a memory barrier in g.Wait() sufficient to not lock the
	// mutex guarding topUpds below this point.

	// Finally, update the topmost shard using the "leaf" shard roots updates.
	rootUpd, err := update(ctx, topUpds)
	if err != nil {
		return nil, err
	}
	return rootUpd.Hash, nil
}

type txAccessor struct {
	// hashes is a cache of node hashes that Get returns directly instead of
	// calling GetMerkleNodes. Used only if preload is true.
	hashes  map[tree.NodeID2][]byte
	preload bool

	tx  storage.MapTreeTX
	rev int64
}

func (t txAccessor) Get(ctx context.Context, ids []tree.NodeID2) (map[tree.NodeID2][]byte, error) {
	// TODO(pavelkalinnikov): Factor out preload into another accessor.
	if t.preload {
		return t.hashes, nil
	}

	// TODO(pavelkalinnikov): Pass NodeID2 directly to storage.
	convIDs := make([]tree.NodeID, 0, len(ids))
	for _, id := range ids {
		convIDs = append(convIDs, tree.NewNodeIDFromID2(id))
	}
	nodes, err := t.tx.GetMerkleNodes(ctx, t.rev-1, convIDs)
	if err != nil {
		return nil, err
	}
	res := make(map[tree.NodeID2][]byte, len(nodes))
	for _, node := range nodes {
		res[node.NodeID.ToNodeID2()] = node.Hash
	}
	return res, nil
}

func (t txAccessor) Set(ctx context.Context, upd []smt.NodeUpdate) error {
	nodes := make([]tree.Node, 0, len(upd))
	for _, u := range upd {
		id := tree.NewNodeIDFromID2(u.ID)
		nodes = append(nodes, tree.Node{NodeID: id, Hash: u.Hash, NodeRevision: t.rev})
	}
	return t.tx.SetMerkleNodes(ctx, nodes)
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

// doPreload causes the subtreeCache in tx to become populated with all
// subtrees on the Merkle path for the indices specified in upd.
// This is a performance workaround for locking issues which occur when the
// sparse Merkle tree code is used with a single transaction (and therefore a
// single subtreeCache too).
func doPreload(ctx context.Context, tx storage.MapTreeTX, depth uint, upd []smt.NodeUpdate, rev int64) (map[tree.NodeID2][]byte, error) {
	ctx, spanEnd := spanFor(ctx, "doPreload")
	defer spanEnd()

	// TODO(pavelkalinnikov): Avoid using HStar3 directly.
	hs, err := smt.NewHStar3(upd, nil, depth, 0)
	if err != nil {
		return nil, err
	}
	acc := txAccessor{tx: tx, rev: rev + 1}
	return acc.Get(ctx, hs.Prepare())
}

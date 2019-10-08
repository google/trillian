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
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
)

// updateTree updates the sparse Merkle tree at the specified revision based on the passed-in
// leaf changes, and writes it to the storage. Returns the new signed map root, which is also
// submitted to storage.
func (t *TrillianMapServer) updateTree(ctx context.Context, tree *trillian.Tree, hasher hashers.MapHasher, tx storage.MapTreeTX, hkv []merkle.HashKeyValue, rev int64) ([]byte, error) {
	// Work around a performance issue when using the map in
	// single-transaction mode by preloading all the nodes we know the
	// sparse Merkle writer is going to need.
	if t.opts.UseSingleTransaction && t.opts.UseLargePreload {
		if err := doPreload(ctx, tx, hasher.BitLen(), hkv); err != nil {
			return nil, err
		}
	}

	smtWriter, err := merkle.NewSparseMerkleTreeWriter(ctx, tree.TreeId, rev, hasher, t.newTXRunner(tree, tx))
	if err != nil {
		return nil, err
	}

	if err = smtWriter.SetLeaves(ctx, hkv); err != nil {
		return nil, err
	}

	rootHash, err := smtWriter.CalculateRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("CalculateRoot(): %v", err)
	}
	return rootHash, nil
}

func (t *TrillianMapServer) newTXRunner(tree *trillian.Tree, tx storage.MapTreeTX) merkle.TXRunner {
	if t.opts.UseSingleTransaction {
		return &singleTXRunner{tx: tx}
	}
	return &multiTXRunner{tree: tree, mapStorage: t.registry.MapStorage}
}

// singleTXRunner executes all calls to Run with the same underlying transaction.
// If f is large, this may incur a performance penalty.
type singleTXRunner struct {
	tx storage.MapTreeTX
}

// RunTX executes a function in the transaction managed by the singleTXRunner.
func (r *singleTXRunner) RunTX(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
	return f(ctx, r.tx)
}

// multiTXRunner executes each call to Run using its own transaction.
// This allows each invocation of f to proceed independently much faster.
// However, If one transaction fails, the other will still succeed (In some cases this could cause data corruption).
type multiTXRunner struct {
	tree       *trillian.Tree
	mapStorage storage.MapStorage
}

// RunTX executes a function in a new transaction.
func (r *multiTXRunner) RunTX(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error {
	return r.mapStorage.ReadWriteTransaction(ctx, r.tree, f)
}

// doPreload causes the subtreeCache in tx to become populated with all subtrees
// on the Merkle path for the indices specified in hkv.
// This is a performance workaround for locking issues which occur when the
// sparse Merkle tree code is used with a single transaction (and therefore
// a single subtreeCache too).
func doPreload(ctx context.Context, tx storage.MapTreeTX, treeDepth int, hkv []merkle.HashKeyValue) error {
	ctx, spanEnd := spanFor(ctx, "doPreload")
	defer spanEnd()

	readRev, err := tx.ReadRevision(ctx)
	if err != nil {
		return err
	}

	nids := calcAllSiblingsParallel(ctx, treeDepth, hkv)
	_, err = tx.GetMerkleNodes(ctx, readRev, nids)
	return err
}

func calcAllSiblingsParallel(_ context.Context, treeDepth int, hkv []merkle.HashKeyValue) []tree.NodeID {
	type nodeAndID struct {
		id   string
		node tree.NodeID
	}
	c := make(chan nodeAndID, 2048)
	var wg sync.WaitGroup

	// Kick off producers.
	for _, i := range hkv {
		wg.Add(1)
		go func(k []byte) {
			defer wg.Done()
			nid := tree.NewNodeIDFromHash(k)
			sibs := nid.Siblings()
			for _, sib := range sibs {
				sibID := sib.AsKey()
				sib := sib
				c <- nodeAndID{sibID, sib}
			}
		}(i.HashedKey)
	}

	// monitor for all the producers being complete to close the channel.
	go func() {
		wg.Wait()
		close(c)
	}()

	nidSet := make(map[string]bool)
	nids := make([]tree.NodeID, 0, len(hkv)*treeDepth)
	// consume the produced IDs until the channel is closed.
	for nai := range c {
		if _, ok := nidSet[nai.id]; !ok {
			nidSet[nai.id] = true
			nids = append(nids, nai.node)
		}
	}

	return nids
}

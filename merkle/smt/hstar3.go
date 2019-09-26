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

// Package smt contains the implementation of the sparse Merkle tree logic.
package smt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/trillian/storage/tree"
)

// NodeStorage reads and writes sparse Merkle tree node hashes.
type NodeStorage interface {
	// Get returns the hash of the given node.
	Get(id tree.NodeID2) ([]byte, error)
	// Set sets the hash of the given node.
	Set(id tree.NodeID2, hash []byte)
}

// HashChildrenFn computes a node hash based on its child nodes' hashes.
type HashChildrenFn func(left, right []byte) []byte

// NodeUpdate represents an update of a node hash in HStar3 algorithm.
type NodeUpdate struct {
	ID   tree.NodeID2
	Hash []byte
}

// HStar3 is a faster non-recursive HStar2.
//
// TODO(pavelkalinnikov): Swap in the code, and document it properly.
type HStar3 struct {
	hash  HashChildrenFn
	depth uint
}

// NewHStar3 returns a new instance of HStar3.
func NewHStar3(hash HashChildrenFn, depth uint) HStar3 {
	return HStar3{hash: hash, depth: depth}
}

// Prepare sorts the updates slice for it to be usable by HStar3. It also
// verifies that the nodes are placed at the required depth, and there are no
// ID duplicates amongh them.
func (h HStar3) Prepare(updates []NodeUpdate) error {
	for i := range updates {
		if d, want := updates[i].ID.BitLen(), h.depth; d != want {
			return fmt.Errorf("upd #%d: invalid depth %d, want %d", i, d, want)
		}
	}
	sort.Slice(updates, func(i, j int) bool {
		return compareHorizontal(updates[i].ID, updates[j].ID)
	})
	for i, last := 0, len(updates)-1; i < last; i++ {
		if id := updates[i].ID; id == updates[i+1].ID {
			return fmt.Errorf("duplicate ID: %v", id)
		}
	}
	return nil
}

// GetReadSet returns the set of all the node IDs that HStar3 requires to read
// in order to apply the passed-in updates and propagate node hash changes up
// to the root(top) node(s) at the specified depth. The updates must have been
// processed by Prepare.
//
// Note: The returned map could have bool value type, but []byte allows the
// caller to reuse this map for filling in the hashes for the Update method.
//
// TODO(pavelkalinnikov): Return only tile IDs.
func (h HStar3) GetReadSet(updates []NodeUpdate, top uint) map[tree.NodeID2][]byte {
	ids := make(map[tree.NodeID2][]byte)
	// For each node, add all its ancestors' siblings, down to the given depth.
	for _, upd := range updates {
		for id, d := upd.ID, h.depth; d > top; d-- {
			sib := id.Prefix(d).Sibling()
			if _, ok := ids[sib]; ok {
				// All the upper siblings have been added already, so skip them.
				break
			}
			ids[sib] = nil
		}
	}
	// Delete the nodes that don't contribute to the updated hashes because
	for id := range ids {
		sib := id.Sibling()
		if _, ok := ids[sib]; ok {
			delete(ids, id)
			delete(ids, sib)
		}
	}
	return ids
}

// Update applies the given updates to a sparse Merkle tree. Requires a
// previous Prepare invocation on the same updates slice. Returns an error if
// any of the NodeStorage.Get calls does so, for example if a node is missing.
//
// Returns the slice of updates at the top level of the sparse Merkle tree
// induced by the provided lower level updates. Typically it will contain only
// one item for the root hash of a tile or a (sub)tree, but the caller may
// arrange multiple subtrees in one call, in which case the corresponding
// returned top-level updates will be sorted lexicographically by node ID.
//
// Note that Update invocations can be chained. For example, a bunch of HStar3
// instances at depth 256 can return updates for depth 8 (in parallel), which
// are then merged together and passed into another HStar3 at depth 8 which
// computes the overall tree root update.
//
// For that reason, Update doesn't invoke NodeStorage.Set for the topmost level
// nodes. If it did then chained Updates would Set the borderline nodes twice.
//
// Warning: This call modifies the updates slice in-place, so the caller must
// ensure to not reuse it.
func (h HStar3) Update(updates []NodeUpdate, top uint, ns NodeStorage) ([]NodeUpdate, error) {
	for d := h.depth; d > top; d-- {
		var err error
		if updates, err = h.updateAt(updates, d, ns); err != nil {
			return nil, fmt.Errorf("depth %d: %v", d, err)
		}
	}
	return updates, nil
}

// updateAt applies the given node updates at the specified tree level.
// Returns the updates that propagated to the level above.
func (h HStar3) updateAt(updates []NodeUpdate, depth uint, ns NodeStorage) ([]NodeUpdate, error) {
	// Apply the updates.
	for _, upd := range updates {
		ns.Set(upd.ID, upd.Hash)
	}
	// Calculate the updates that propagate to one level above.
	newLen := 0
	for i, ln := 0, len(updates); i < ln; i, newLen = i+1, newLen+1 {
		sib := updates[i].ID.Sibling()
		left, right := updates[i].Hash, []byte{}
		if next := i + 1; next < ln && updates[next].ID == sib {
			// The sibling is the right child here, as updates are sorted.
			right = updates[next].Hash
			i = next // Skip the next update in the outer loop.
		} else {
			// The sibling is not updated, so fetch the original from NodeStorage.
			var err error
			if right, err = ns.Get(sib); err != nil {
				return nil, err
			}
			if isLeftChild(sib) {
				left, right = right, left
			}
		}
		hash := h.hash(left, right)
		updates[newLen] = NodeUpdate{ID: sib.Prefix(depth - 1), Hash: hash}
	}
	return updates[:newLen], nil
}

// isLeftChild returns whether the the given node is a left child.
func isLeftChild(id tree.NodeID2) bool {
	last, bits := id.LastByte()
	return last&(1<<(8-bits)) == 0
}

// compareHorizontal returns whether the first node ID is to the left from the
// second one. The result only makes sense for IDs at the same tree level.
func compareHorizontal(a, b tree.NodeID2) bool {
	if res := strings.Compare(a.FullBytes(), b.FullBytes()); res != 0 {
		return res < 0
	}
	aLast, _ := a.LastByte()
	bLast, _ := b.LastByte()
	return aLast < bLast
}

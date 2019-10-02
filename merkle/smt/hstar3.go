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

// NodeAccessor provides read and write access to Merkle tree node hashes.
//
// The Update algorithm uses it to read the existing nodes of the tree and
// write the nodes that are updated. The nodes that it writes do not intersect
// with the nodes that it reads.
type NodeAccessor interface {
	// Get returns the hash of the given node. Returns an error if the hash is
	// undefined, or can't be obtained for another reason.
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
	upd   []NodeUpdate
	hash  HashChildrenFn
	depth uint
	top   uint
}

// NewHStar3 returns a new instance of HStar3 for the given set of node hash
// updates at the specified tree depth. This HStar3 is capable of propagating
// these updates up to the passed-in top level of the tree.
//
// Warning: This call and other HStar3 methods modify the updates slice
// in-place, so the caller must ensure to not reuse it.
func NewHStar3(updates []NodeUpdate, hash HashChildrenFn, depth, top uint) (HStar3, error) {
	if err := sortUpdates(updates, depth); err != nil {
		return HStar3{}, err
	} else if top > depth {
		return HStar3{}, fmt.Errorf("top > depth: %d vs. %d", top, depth)
	}
	return HStar3{upd: updates, hash: hash, depth: depth, top: top}, nil
}

// Prepare returns the list of all the node IDs that the Update method will
// load in order to compute node hash updates from the initial tree depth up to
// the top level specified in the constructor. The ordering of the returned IDs
// is arbitrary. This method may be useful for creating a NodeAccessor, e.g.
// by batch-reading the nodes from elsewhere.
//
// TODO(pavelkalinnikov): Return only tile IDs.
func (h HStar3) Prepare() []tree.NodeID2 {
	empty := tree.NodeID2{}

	// Start with a single "sentinel" empty ID, which helps maintaining the loop
	// invariants below. Preallocate enough memory to store all the node IDs.
	ids := make([]tree.NodeID2, 1, len(h.upd)*int(h.depth-h.top)+1)
	pos := make([]int, h.depth-h.top)

	// For each node, add all its ancestors' siblings, down to the given depth.
	// Avoid duplicate IDs, and possibly remove already added ones if they become
	// unnecessary as more updates are added.
	//
	// Loop invariants:
	// 1. pos[idx] < len(ids), for each idx.
	// 2. ids[pos[idx]] is the ID of the rightmost sibling at depth idx+h.top+1,
	//    or an empty ID if there is none at this depth yet.
	//
	// Note: The algorithm works because the list of updates is sorted.
	for _, upd := range h.upd {
		for id, d := upd.ID, h.depth; d > h.top; d-- {
			pref := id.Prefix(d)
			idx := d - h.top - 1
			if p := pos[idx]; ids[p] == pref {
				// Delete that node because its original hash will be overridden, so it
				// does not contribute to hash updates anymore.
				ids[p] = empty
				// Skip the upper siblings as they have been added already.
				break
			}
			pos[idx] = len(ids)
			ids = append(ids, pref.Sibling())
		}
	}

	// Delete all empty IDs, which include the 0-th "sentinel" ID and the ones
	// that were marked as such in the loop above.
	newLen := 0
	for i := range ids {
		if ids[i] != empty {
			ids[newLen] = ids[i]
			newLen++
		}
	}
	return ids[:newLen]
}

// Update applies the updates to the sparse Merkle tree. Returns an error if
// any of the NodeAccessor.Get calls does so, e.g. if a node is undefined.
// Warning: HStar3 must not be used further after this call.
//
// Returns the slice of updates at the top level of the sparse Merkle tree
// induced by the provided lower level updates. Typically it will contain only
// one item for the root hash of a tile or a (sub)tree, but the caller may
// arrange multiple subtrees in one HStar3, in which case the corresponding
// returned top-level updates will be sorted lexicographically by node ID.
//
// Note that Update invocations can be chained. For example, a bunch of HStar3
// instances at depth 256 can return updates for depth 8 (in parallel), which
// are then merged together and passed into another HStar3 at depth 8 which
// computes the overall tree root update.
//
// For that reason, Update doesn't invoke NodeAccessor.Set for the topmost
// nodes. If it did then chained Updates would Set the borderline nodes twice.
func (h HStar3) Update(na NodeAccessor) ([]NodeUpdate, error) {
	for d := h.depth; d > h.top; d-- {
		var err error
		if h.upd, err = h.updateAt(h.upd, d, na); err != nil {
			return nil, fmt.Errorf("depth %d: %v", d, err)
		}
	}
	return h.upd, nil
}

// updateAt applies the given node updates at the specified tree level.
// Returns the updates that propagated to the level above.
func (h HStar3) updateAt(updates []NodeUpdate, depth uint, na NodeAccessor) ([]NodeUpdate, error) {
	// Apply the updates.
	for _, upd := range updates {
		na.Set(upd.ID, upd.Hash)
	}
	// Calculate the updates that propagate to one level above. The result of
	// this is a slice of newLen items, between len/2 and len. The length shrinks
	// whenever two updated nodes share the same parent.
	newLen := 0
	for i, ln := 0, len(updates); i < ln; i++ {
		sib := updates[i].ID.Sibling()
		var left, right []byte
		if next := i + 1; next < ln && updates[next].ID == sib {
			// The sibling is the right child here, as updates are sorted.
			left, right = updates[i].Hash, updates[next].Hash
			i = next // Skip the next update in the outer loop.
		} else {
			// The sibling is not updated, so fetch the original from NodeAccessor.
			hash, err := na.Get(sib)
			if err != nil {
				return nil, err
			}
			left, right = updates[i].Hash, hash
			if isLeftChild(sib) {
				left, right = right, left
			}
		}
		hash := h.hash(left, right)
		updates[newLen] = NodeUpdate{ID: sib.Prefix(depth - 1), Hash: hash}
		newLen++
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

// sortUpdates sorts the updates slice for it to be usable by HStar3. It also
// verifies that the nodes are placed at the required depth, and there are no
// duplicate IDs.
func sortUpdates(updates []NodeUpdate, depth uint) error {
	for i := range updates {
		if d, want := updates[i].ID.BitLen(), depth; d != want {
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

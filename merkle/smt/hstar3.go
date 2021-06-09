// Copyright 2019 Google LLC. All Rights Reserved.
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

	"github.com/google/trillian/merkle/smt/node"
)

// NodeAccessor provides read and write access to Merkle tree node hashes.
//
// The Update algorithm uses it to read the existing nodes of the tree and
// write the nodes that are updated. The nodes that it writes do not intersect
// with the nodes that it reads.
type NodeAccessor interface {
	// Get returns the hash of the given node. Returns an error if the hash is
	// undefined, or can't be obtained for another reason.
	Get(id node.ID) ([]byte, error)
	// Set sets the hash of the given node.
	Set(id node.ID, hash []byte)
}

// HashChildrenFn computes a node hash based on its child nodes' hashes.
type HashChildrenFn func(left, right []byte) []byte

// HStar3 is a faster non-recursive HStar2.
type HStar3 struct {
	nodes []Node
	hash  HashChildrenFn
	depth uint
	top   uint
}

// NewHStar3 returns a new instance of HStar3 for the given set of node hash
// updates at the specified tree depth. This HStar3 is capable of propagating
// these updates up to the passed-in top level of the tree.
//
// Warning: This call and other HStar3 methods modify the nodes slice in-place,
// so the caller must ensure to not reuse it.
func NewHStar3(nodes []Node, hash HashChildrenFn, depth, top uint) (HStar3, error) {
	if err := Prepare(nodes, depth); err != nil {
		return HStar3{}, err
	} else if top > depth {
		return HStar3{}, fmt.Errorf("top > depth: %d vs. %d", top, depth)
	}
	return HStar3{nodes: nodes, hash: hash, depth: depth, top: top}, nil
}

// Prepare returns the list of all the node IDs that the Update method will
// load in order to compute node hash updates from the initial tree depth up to
// the top level specified in the constructor. The ordering of the returned IDs
// is arbitrary. This method may be useful for creating a NodeAccessor, e.g.
// by batch-reading the nodes from elsewhere.
//
// TODO(pavelkalinnikov): Return only tile IDs.
func (h HStar3) Prepare() []node.ID {
	// Start with a single "sentinel" empty ID, which helps maintaining the loop
	// invariants below. Preallocate enough memory to store all the node IDs.
	ids := make([]node.ID, 1, len(h.nodes)*int(h.depth-h.top)+1)
	pos := make([]int, h.depth-h.top)
	// Note: This variable compares equal to ids[0].
	empty := node.ID{}

	// For each node, add all its ancestors' siblings, down to the given depth.
	// Avoid duplicate IDs, and possibly remove already added ones if they become
	// unnecessary as more updates are added.
	//
	// Loop invariants:
	// 1. pos[idx] < len(ids), for each idx.
	// 2. ids[pos[idx]] is the ID of the rightmost sibling at depth idx+h.top+1
	//    added so far, or an empty ID if there is none at this depth yet.
	//
	// Note: The algorithm works because the list of updates is sorted.
	for _, n := range h.nodes {
		for id, d := n.ID, h.depth; d > h.top; d-- {
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
func (h HStar3) Update(na NodeAccessor) ([]Node, error) {
	for d := h.depth; d > h.top; d-- {
		var err error
		if h.nodes, err = h.updateAt(h.nodes, d, na); err != nil {
			return nil, fmt.Errorf("depth %d: %v", d, err)
		}
	}
	return h.nodes, nil
}

// updateAt applies the given node updates at the specified tree level.
// Returns the updates that propagated to the level above.
func (h HStar3) updateAt(nodes []Node, depth uint, na NodeAccessor) ([]Node, error) {
	// Apply the updates.
	for _, n := range nodes {
		na.Set(n.ID, n.Hash)
	}
	// Calculate the updates that propagate to one level above. The result of
	// this is a slice of newLen items, between len/2 and len. The length shrinks
	// whenever two updated nodes share the same parent.
	newLen := 0
	for i, ln := 0, len(nodes); i < ln; i++ {
		sib := nodes[i].ID.Sibling()
		var left, right []byte
		if next := i + 1; next < ln && nodes[next].ID == sib {
			// The sibling is the right child here, as nodes are sorted.
			left, right = nodes[i].Hash, nodes[next].Hash
			i = next // Skip the next update in the outer loop.
		} else {
			// The sibling is not updated, so fetch the original from NodeAccessor.
			hash, err := na.Get(sib)
			if err != nil {
				return nil, err
			}
			left, right = nodes[i].Hash, hash
			if isLeftChild(sib) {
				left, right = right, left
			}
		}
		hash := h.hash(left, right)
		nodes[newLen] = Node{ID: sib.Prefix(depth - 1), Hash: hash}
		newLen++
	}
	return nodes[:newLen], nil
}

// isLeftChild returns whether the given node is a left child.
func isLeftChild(id node.ID) bool {
	last, bits := id.LastByte()
	return last&(1<<(8-bits)) == 0
}

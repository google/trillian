// Copyright 2021 Google LLC. All Rights Reserved.
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

// Package proof contains helpers for constructing log Merkle tree proofs.
package proof

import (
	"math/bits"

	"github.com/google/trillian/merkle/compact"
)

// NodeFetch bundles a node ID with additional information on how to use the
// node to construct a proof.
type NodeFetch struct {
	ID     compact.NodeID
	Rehash bool
}

// Consistency returns node addresses for the consistency proof between the
// given tree sizes.
func Consistency(size1, size2 uint64) []NodeFetch {
	if size1 == size2 {
		return []NodeFetch{}
	}

	// TODO(pavelkalinnikov): Make the capacity estimate accurate.
	proof := make([]NodeFetch, 0, bits.Len64(size2)+1)

	// Find the biggest perfect subtree that ends at size1.
	level := uint(bits.TrailingZeros64(size1))
	index := (size1 - 1) >> level
	// If it does not cover the whole size1 tree, add this node to the proof.
	if index != 0 {
		n := compact.NewNodeID(level, index)
		proof = append(proof, NodeFetch{ID: n})
	}

	// Now append the path from this node to the root of size2.
	p := Nodes(index, level, size2, true)
	return append(proof, p...)
}

// Nodes returns the node IDs necessary to prove that the (level, index) node
// is included in the Merkle tree of the given size.
func Nodes(index uint64, level uint, size uint64, rehash bool) []NodeFetch {
	// [begin, end) is the leaves range covered by the (level, index) node.
	begin, end := index<<level, (index+1)<<level
	// To prove inclusion of range [begin, end), we only need nodes of compact
	// range [0, begin) and [end, size). Further down, we need the nodes ordered
	// by level from leaves towards the root.
	left := reverse(compact.RangeNodes(0, begin))
	// We decompose the [end, size) range into [end, end+l) and [end+l, size).
	// The first one (named `middle` here) contains all the nodes that don't have
	// a left sibling within [end, size), and the second one (named `right`
	// below) contains all the nodes that don't have a right sibling.
	l, r := compact.Decompose(end, size)
	middle := compact.RangeNodes(end, end+l)

	// Nodes that don't have a right sibling (i.e. the right border of the tree)
	// are special, because their hashes are collapsed into a single "ephemeral"
	// hash. This hash is already known if rehash==false, otherwise the caller
	// needs to compute it based on the hashes of compact range [end+l, size).
	//
	// TODO(pavelkalinnikov): Always assume rehash = true.
	var right []compact.NodeID
	if r != 0 {
		if rehash {
			right = reverse(compact.RangeNodes(end+l, size))
			rehash = len(right) > 1
		} else {
			// The parent of the highest node in [end+l, size) is "ephemeral".
			lvl := uint(bits.Len64(r))
			// Except when [end+l, size) is a perfect subtree, in which case we just
			// take the root node.
			if r&(r-1) == 0 {
				lvl--
			}
			right = []compact.NodeID{compact.NewNodeID(lvl, (end+l)>>lvl)}
		}
	}

	// The level in the ordered list of nodes where the rehashed nodes appear in
	// lieu of the "ephemeral" node. This is equal to the level where the path to
	// the `begin` index diverges from the path to `size`.
	rehashLevel := uint(bits.Len64(begin^size) - 1)

	// Merge the three compact ranges into a single proof ordered by node level
	// from leaves towards the root, i.e. the format specified in RFC 6962.
	proof := make([]NodeFetch, 0, len(left)+len(middle)+len(right))
	i, j := 0, 0
	for l, levels := level, uint(bits.Len64(size-1)); l < levels; l++ {
		if i < len(left) && left[i].Level == l {
			proof = append(proof, NodeFetch{ID: left[i]})
			i++
		} else if j < len(middle) && middle[j].Level == l {
			proof = append(proof, NodeFetch{ID: middle[j]})
			j++
		}
		if l == rehashLevel {
			for _, id := range right {
				proof = append(proof, NodeFetch{ID: id, Rehash: rehash})
			}
		}
	}

	return proof
}

func reverse(ids []compact.NodeID) []compact.NodeID {
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

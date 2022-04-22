// Copyright 2016 Google LLC. All Rights Reserved.
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

package merkle

import (
	"errors"
	"math/bits"

	"github.com/google/trillian/merkle/compact" // nolint:staticcheck
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeFetch bundles a node ID with additional information on how to use the
// node to construct a proof.
type NodeFetch struct {
	ID     compact.NodeID
	Rehash bool
}

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to build an
// inclusion proof for a specified tree size and leaf index. All the returned
// nodes represent complete subtrees in the tree of this size or above.
//
// Use Rehash function to compose the proof after the node hashes are fetched.
func CalcInclusionProofNodeAddresses(size, index int64) ([]NodeFetch, error) {
	if size < 1 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: size %d < 1", size)
	}
	if index >= size {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: index %d is >= size %d", index, size)
	}
	if index < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for inclusion proof: index %d is < 0", index)
	}
	return proofNodes(uint64(index), 0, uint64(size), true), nil
}

// CalcConsistencyProofNodeAddresses returns the tree node IDs needed to build
// a consistency proof between two specified tree sizes. All the returned nodes
// represent complete subtrees in the tree of size2 or above.
//
// Use Rehash function to compose the proof after the node hashes are fetched.
func CalcConsistencyProofNodeAddresses(size1, size2 int64) ([]NodeFetch, error) {
	if size1 < 1 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size1 %d < 1", size1)
	}
	if size2 < 1 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size2 %d < 1", size2)
	}
	if size1 > size2 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size1 %d > size2 %d", size1, size2)
	}

	return consistencyNodes(uint64(size1), uint64(size2)), nil
}

// consistencyNodes returns node addresses for the consistency proof between
// the given tree sizes.
func consistencyNodes(size1, size2 uint64) []NodeFetch {
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
	p := proofNodes(index, level, size2, true)
	return append(proof, p...)
}

// proofNodes returns the node IDs necessary to prove that the (level, index)
// node is included in the Merkle tree of the given size.
func proofNodes(index uint64, level uint, size uint64, rehash bool) []NodeFetch {
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

// Rehash computes the proof based on the slice of NodeFetch structs, and the
// corresponding hashes of these nodes. The slices must be of the same length.
// The hc parameter computes node's hash based on hashes of its children.
//
// Warning: The passed-in slice of hashes can be modified in-place.
func Rehash(h [][]byte, nf []NodeFetch, hc func(left, right []byte) []byte) ([][]byte, error) {
	if len(h) != len(nf) {
		return nil, errors.New("slice lengths mismatch")
	}
	cursor := 0
	// Scan the list of node hashes, and store the rehashed list in-place.
	// Invariant: cursor <= i, and h[:cursor] contains all the hashes of the
	// rehashed list after scanning h up to index i-1.
	for i, ln := 0, len(h); i < ln; i, cursor = i+1, cursor+1 {
		hash := h[i]
		if nf[i].Rehash {
			// Scan the block of node hashes that need rehashing.
			for i++; i < len(nf) && nf[i].Rehash; i++ {
				hash = hc(h[i], hash)
			}
			i--
		}
		h[cursor] = hash
	}
	return h[:cursor], nil
}

func reverse(ids []compact.NodeID) []compact.NodeID {
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

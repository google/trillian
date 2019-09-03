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

package merkle

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
)

var (
	// ErrSubtreeOverrun indicates that a subtree exceeds the maximum tree depth.
	ErrSubtreeOverrun = errors.New("subtree with prefix exceeds maximum tree size")
	smtOne            = big.NewInt(1)
	smtZero           = big.NewInt(0)
)

// HStar2LeafHash represents a leaf for the HStar2 sparse Merkle tree
// implementation.
type HStar2LeafHash struct {
	// TODO(al): remove big.Int
	Index    *big.Int
	LeafHash []byte
}

// HStar2 is a recursive implementation for calculating the root hash of a sparse
// Merkle tree.
type HStar2 struct {
	treeID int64
	hasher hashers.MapHasher
}

// NewHStar2 creates a new HStar2 tree calculator based on the passed in MapHasher.
func NewHStar2(treeID int64, hasher hashers.MapHasher) HStar2 {
	return HStar2{
		treeID: treeID,
		hasher: hasher,
	}
}

// HStar2Root calculates the root of a sparse Merkle tree of a given depth
// which contains the given set of non-null leaves.
func (s *HStar2) HStar2Root(depth int, values []*HStar2LeafHash) ([]byte, error) {
	sort.Sort(ByIndex{values})
	combine := func(depth int, offset *big.Int, lhs, rhs []byte) ([]byte, error) {
		h := s.hasher.HashChildren(lhs, rhs)
		return h, nil
	}
	return s.hStar2b(0, depth, values, smtZero, nil, combine)
}

// SparseGetNodeFunc should return any pre-existing node hash for the node address.
type SparseGetNodeFunc func(depth int, index *big.Int) ([]byte, error)

// SparseSetNodeFunc should store the passed node hash, associating it with the address.
type SparseSetNodeFunc func(depth int, index *big.Int, hash []byte) error

// HStar2Nodes calculates the root hash of a pre-existing sparse Merkle tree
// plus the extra values passed in.  Get and set are used to fetch and store
// internal node values. Values must not contain multiple leaves for the same
// index.
//
// prefix is the location of this subtree within the larger tree. Root is at nil.
// subtreeDepth is the number of levels in this subtree.
func (s *HStar2) HStar2Nodes(prefix []byte, subtreeDepth int, values []*HStar2LeafHash,
	get SparseGetNodeFunc, set SparseSetNodeFunc) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("HStar2Nodes(%x, %v, %v)", prefix, subtreeDepth, len(values))
		for _, v := range values {
			glog.Infof("  %x: %x", v.Index.Bytes(), v.LeafHash)
		}
	}
	combine := func(depth int, offset *big.Int, lhs, rhs []byte) ([]byte, error) {
		h := s.hasher.HashChildren(lhs, rhs)
		if err := s.set(offset, depth, h, set); err != nil {
			return nil, err
		}
		return h, nil
	}
	return s.run(prefix, subtreeDepth, values, get, combine)
}

// Prefetch does a dry run of HStar2 algorithm, and reports all Merkle tree
// nodes that it needs through the passed-in visit function. Note that the
// return value of the visit function is ignored, unless it is an error.
//
// This function can be useful, for example, if the caller prefers to collect
// the node IDs and read them from storage in one batch. Then they can run
// HStar2Nodes in such a way that it reads from the prefetched set.
func (s *HStar2) Prefetch(prefix []byte, subtreeDepth int, values []*HStar2LeafHash, visit SparseGetNodeFunc) error {
	combine := func(depth int, offset *big.Int, lhs, rhs []byte) ([]byte, error) {
		return nil, nil
	}
	_, err := s.run(prefix, subtreeDepth, values, visit, combine)
	return err
}

// combineFunc returns a node hash based on two child hashes. It may also do
// side effects, e.g. put the resulting node to storage.
type combineFunc func(depth int, offset *big.Int, lhs, rhs []byte) ([]byte, error)

// run runs the HStar2 algorithm.
func (s *HStar2) run(prefix []byte, subtreeDepth int, values []*HStar2LeafHash,
	get SparseGetNodeFunc, combine combineFunc) ([]byte, error) {
	depth := len(prefix) * 8
	totalDepth := depth + subtreeDepth
	if totalDepth > s.hasher.BitLen() {
		return nil, ErrSubtreeOverrun
	}
	sort.Sort(ByIndex{values})
	offset := storage.NewNodeIDFromPrefixSuffix(prefix, storage.EmptySuffix, s.hasher.BitLen()).BigInt()
	return s.hStar2b(depth, totalDepth, values, offset, get, combine)
}

// hStar2b computes a sparse Merkle tree root value recursively.
func (s *HStar2) hStar2b(depth, maxDepth int, values []*HStar2LeafHash, offset *big.Int,
	get SparseGetNodeFunc, combine combineFunc) ([]byte, error) {
	if depth == maxDepth {
		switch {
		case len(values) == 0:
			return s.get(offset, depth, get)
		case len(values) == 1:
			return values[0].LeafHash, nil
		default:
			glog.Errorf("base case with too many values: %+v", values)
			return nil, fmt.Errorf("hStar2b base case: len(values): %d, want 1", len(values))
		}
	}
	if len(values) == 0 {
		return s.get(offset, depth, get)
	}

	bitsLeft := s.hasher.BitLen() - depth
	split := new(big.Int).Lsh(smtOne, uint(bitsLeft-1))
	split.Add(split, offset)
	i := sort.Search(len(values), func(i int) bool { return values[i].Index.Cmp(split) >= 0 })
	lhs, err := s.hStar2b(depth+1, maxDepth, values[:i], offset, get, combine)
	if err != nil {
		return nil, err
	}
	rhs, err := s.hStar2b(depth+1, maxDepth, values[i:], split, get, combine)
	if err != nil {
		return nil, err
	}
	return combine(depth, offset, lhs, rhs)
}

// get attempts to use getter. If getter fails, returns the HashEmpty value.
func (s *HStar2) get(index *big.Int, depth int, getter SparseGetNodeFunc) ([]byte, error) {
	// if we've got a function for getting existing node values, try it:
	if getter != nil {
		h, err := getter(depth, index)
		if err != nil {
			return nil, err
		}
		// if we got a value then we'll use that
		if h != nil {
			return h, nil
		}
	}
	// TODO(gdbelvin): Hashers should accept depth as their main argument.
	height := s.hasher.BitLen() - depth
	nodeID := storage.NewNodeIDFromBigInt(index.BitLen(), index, s.hasher.BitLen())
	return s.hasher.HashEmpty(s.treeID, nodeID.Path, height), nil
}

// set attempts to use setter if it not nil.
func (s *HStar2) set(index *big.Int, depth int, hash []byte, setter SparseSetNodeFunc) error {
	if setter != nil {
		return setter(depth, index, hash)
	}
	return nil
}

// HStar2LeafHash sorting boilerplate below.

// Leaves is a slice of HStar2LeafHash
type Leaves []*HStar2LeafHash

// Len returns the number of leaves.
func (s Leaves) Len() int { return len(s) }

// Swap swaps two leaf locations.
func (s Leaves) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// ByIndex implements sort.Interface by providing Less and using Len and Swap methods from the embedded Leaves value.
type ByIndex struct{ Leaves }

// Less returns true if i.Index < j.Index
func (s ByIndex) Less(i, j int) bool { return s.Leaves[i].Index.Cmp(s.Leaves[j].Index) < 0 }

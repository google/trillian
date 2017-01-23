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
)

var (
	// ErrNegativeTreeLevelOffset indicates a negative level was specified.
	ErrNegativeTreeLevelOffset = errors.New("treeLevelOffset cannot be negative")
)

// HStar2LeafHash represents a leaf for the HStar2 sparse Merkle tree
// implementation.
type HStar2LeafHash struct {
	// TODO(al): remove big.Int
	Index    *big.Int
	LeafHash []byte
}

// HStar2 is a recursive implementation for calulating the root hash of a sparse
// Merkle tree.
type HStar2 struct {
	hasher          TreeHasher
	hStarEmptyCache [][]byte
}

// NewHStar2 creates a new HStar2 tree calculator based on the passed in
// TreeHasher.
func NewHStar2(treeHasher TreeHasher) HStar2 {
	return HStar2{
		hasher:          treeHasher,
		hStarEmptyCache: [][]byte{treeHasher.HashLeaf([]byte(""))},
	}
}

// HStar2Root calculates the root of a sparse Merkle tree of depth n which contains
// the given set of non-null leaves.
func (s *HStar2) HStar2Root(n int, values []HStar2LeafHash) ([]byte, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.hStar2b(n, values, offset,
		func(depth int, index *big.Int) ([]byte, error) { return s.hStarEmpty(depth) },
		func(int, *big.Int, []byte) error { return nil })
}

// SparseGetNodeFunc should return any pre-existing node hash for the node address.
type SparseGetNodeFunc func(depth int, index *big.Int) ([]byte, error)

// SparseSetNodeFunc should store the passed node hash, associating it with the address.
type SparseSetNodeFunc func(depth int, index *big.Int, hash []byte) error

// HStar2Nodes calculates the root hash of a pre-existing sparse Merkle tree
// plus the extra values passed in.
// It uses the get and set functions to fetch and store updated internal node
// values.
//
// The treeLevelOffset argument is used when the tree to be calculated is part
// of a larger tree. It identifes the level in the larger tree at which the
// root of the subtree being calculated is found.
// e.g. Imagine a tree 256 levels deep, and that you already (somehow) happen
// to have the intermediate hash values for the non-null nodes 8 levels below
// the root already calculated (i.e. you just need to calculate the top 8
// levels of a 256-level tree).  To do this, you'd set treeDepth=8, and
// treeLevelOffset=248 (256-8).
func (s *HStar2) HStar2Nodes(treeDepth, treeLevelOffset int, values []HStar2LeafHash, get SparseGetNodeFunc, set SparseSetNodeFunc) ([]byte, error) {
	if treeLevelOffset < 0 {
		return nil, ErrNegativeTreeLevelOffset
	}
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.hStar2b(treeDepth, values, offset,
		func(depth int, index *big.Int) ([]byte, error) {
			// if we've got a function for getting existing node values, try it:
			h, err := get(treeDepth-depth, index)
			if err != nil {
				return nil, err
			}
			// if we got a value then we'll use that
			if h != nil {
				return h, nil
			}
			// otherwise just return the null hash for this level
			return s.hStarEmpty(depth + treeLevelOffset)
		},
		func(depth int, index *big.Int, hash []byte) error {
			return set(treeDepth-depth, index, hash)
		})
}

// hStarEmpty calculates (and caches) the "null-hash" for the requested tree
// level.
func (s *HStar2) hStarEmpty(n int) ([]byte, error) {
	if len(s.hStarEmptyCache) <= n {
		emptyRoot, err := s.hStarEmpty(n - 1)
		if err != nil {
			return nil, err
		}
		h := s.hasher.HashChildren(emptyRoot, emptyRoot)
		if len(s.hStarEmptyCache) != n {
			return nil, fmt.Errorf("cache wrong size - expected %d, but cache contains %d entries", n, len(s.hStarEmptyCache))
		}
		s.hStarEmptyCache = append(s.hStarEmptyCache, h)
	}
	if n >= len(s.hStarEmptyCache) {
		return nil, fmt.Errorf("cache too small - want level %d, but cache only contains %d entries", n, len(s.hStarEmptyCache))
	}
	return s.hStarEmptyCache[n], nil
}

var (
	smtOne = big.NewInt(1)
)

// hStar2b is the recursive implementation for calculating a sparse Merkle tree
// root value.
func (s *HStar2) hStar2b(n int, values []HStar2LeafHash, offset *big.Int, get SparseGetNodeFunc, set SparseSetNodeFunc) ([]byte, error) {
	if n == 0 {
		switch {
		case len(values) == 0:
			return get(n, offset)
		case len(values) != 1:
			return nil, fmt.Errorf("expected 1 value remaining, but found %d", len(values))
		}
		return values[0].LeafHash, nil
	}

	split := new(big.Int).Lsh(smtOne, uint(n-1))
	split.Add(split, offset)
	if len(values) == 0 {
		return get(n, offset)
	}

	i := sort.Search(len(values), func(i int) bool { return values[i].Index.Cmp(split) >= 0 })
	lhs, err := s.hStar2b(n-1, values[:i], offset, get, set)
	if err != nil {
		return nil, err
	}
	rhs, err := s.hStar2b(n-1, values[i:], split, get, set)
	if err != nil {
		return nil, err
	}
	h := s.hasher.HashChildren(lhs, rhs)
	if set != nil {
		set(n, offset, h)
	}
	return h, nil
}

// HStar2LeafHash sorting boilerplate below.

type by func(a, b *HStar2LeafHash) bool

func (by by) Sort(values []HStar2LeafHash) {
	s := &valueSorter{
		values: values,
		by:     by,
	}
	sort.Sort(s)
}

type valueSorter struct {
	values []HStar2LeafHash
	by     func(a, b *HStar2LeafHash) bool
}

func (s *valueSorter) Len() int {
	return len(s.values)
}

func (s *valueSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
}

func (s *valueSorter) Less(i, j int) bool {
	return s.by(&s.values[i], &s.values[j])
}

func indexLess(a, b *HStar2LeafHash) bool {
	return a.Index.Cmp(b.Index) < 0
}

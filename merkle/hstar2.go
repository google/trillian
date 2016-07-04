package merkle

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/google/trillian"
)

// HStar2LeafHash represents a leaf for the HStar2 sparse merkle tree
// implementation.
type HStar2LeafHash struct {
	// TODO(al): remove big.Int
	Index    *big.Int
	LeafHash trillian.Hash
}

// HStar2 is a recursive implementation for calulating the root hash of a sparse
// merkle tree.
type HStar2 struct {
	hasher          TreeHasher
	hStarEmptyCache []trillian.Hash
}

// NewHStar2 creates a new HStar2 tree calculator based on the passed in
// TreeHasher.
func NewHStar2(treeHasher TreeHasher) HStar2 {
	return HStar2{
		hasher:          treeHasher,
		hStarEmptyCache: []trillian.Hash{treeHasher.HashLeaf([]byte(""))},
	}
}

// HStar2Root calculates the root of a sparse merkle tree of depth n which contains
// the given set of non-null leaves.
func (s *HStar2) HStar2Root(n int, values []HStar2LeafHash) (trillian.Hash, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.hStar2b(n, values, offset,
		func(depth int, index *big.Int) (trillian.Hash, error) { return s.hStarEmpty(depth) },
		func(int, *big.Int, trillian.Hash) error { return nil })
}

// SparseGetNodeFunc should return any pre-existing node hash for the node address.
type SparseGetNodeFunc func(depth int, index *big.Int) (trillian.Hash, error)

// SparseSetNodeFunc should store the passed node hash, associating it with the address.
type SparseSetNodeFunc func(depth int, index *big.Int, hash trillian.Hash) error

// HStar2Nodes calculates the root hash of a pre-existing sparse merkle tree
// plus the extra values passed in.
// It uses the get and set functions to fetch and store updated internal node
// values.
func (s *HStar2) HStar2Nodes(n int, values []HStar2LeafHash, get SparseGetNodeFunc, set SparseSetNodeFunc) (trillian.Hash, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.hStar2b(n, values, offset,
		func(depth int, index *big.Int) (trillian.Hash, error) {
			// if we've got a function for getting existing node values, try it:
			h, err := get(n-depth, index)
			if err != nil {
				return nil, err
			}
			// if we got a value then we'll use that
			if h != nil {
				return h, nil
			}
			// otherwise just return the null hash for this level
			return s.hStarEmpty(depth)
		},
		func(depth int, index *big.Int, hash trillian.Hash) error { return set(n-depth, index, hash) })
}

// hStarEmpty calculates (and caches) the "null-hash" for the requested tree
// level.
func (s *HStar2) hStarEmpty(n int) (trillian.Hash, error) {
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

// hStar2b is the recursive implementation for calculating a sparse merkle tree
// root value.
func (s *HStar2) hStar2b(n int, values []HStar2LeafHash, offset *big.Int, get SparseGetNodeFunc, set SparseSetNodeFunc) (trillian.Hash, error) {

	if n == 0 {
		switch {
		case len(values) == 0:
			return get(n, big.NewInt(0))
		case len(values) != 1:
			return nil, fmt.Errorf("expected 1 value remaining, but found %d", len(values))
		}
		return values[0].LeafHash, nil
	}

	split := new(big.Int).Lsh(big.NewInt(1), uint(n-1))
	split.Add(split, offset)
	if len(values) == 0 {
		return get(n, split)
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
		set(n, split, h)
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

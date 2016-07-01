package merkle

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/google/trillian"
)

// HStar2Value represents a leaf for the HStar2 sparse merkle tree
// implementation.
type HStar2Value struct {
	// TODO(al): remove big.Int
	Index *big.Int
	Value trillian.Hash
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

// HStar2 calculates the root of a sparse merkle tree of depth n which contains
// the given set of non-null leaves.
func (s *HStar2) HStar2(n int, values []HStar2Value) (trillian.Hash, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.hStar2b(n, values, offset)
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
		return nil, fmt.Errorf("cache wrong size - expected %d or more, but cache contains %d entries", n, len(s.hStarEmptyCache))
	}
	return s.hStarEmptyCache[n], nil
}

// hStar2b is the recursive implementation for calculating a sparse merkle tree
// root value.
func (s *HStar2) hStar2b(n int, values []HStar2Value, offset *big.Int) (trillian.Hash, error) {
	if n == 0 {
		switch {
		case len(values) == 0:
			return s.hStarEmptyCache[0], nil
		case len(values) != 1:
			return nil, fmt.Errorf("expected 1 value remaining, but found %d", len(values))
		}
		return s.hasher.HashLeaf(values[0].Value), nil
	}
	if len(values) == 0 {
		return s.hStarEmpty(n)
	}

	split := new(big.Int).Lsh(big.NewInt(1), uint(n-1))
	split.Add(split, offset)
	i := sort.Search(len(values), func(i int) bool { return values[i].Index.Cmp(split) >= 0 })
	lhs, err := s.hStar2b(n-1, values[:i], offset)
	if err != nil {
		return nil, err
	}
	rhs, err := s.hStar2b(n-1, values[i:], split)
	if err != nil {
		return nil, err
	}
	return s.hasher.HashChildren(lhs, rhs), nil
}

// HStar2Value sorting boilerplate below.

type by func(a, b *HStar2Value) bool

func (by by) Sort(values []HStar2Value) {
	s := &valueSorter{
		values: values,
		by:     by,
	}
	sort.Sort(s)
}

type valueSorter struct {
	values []HStar2Value
	by     func(a, b *HStar2Value) bool
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

func indexLess(a, b *HStar2Value) bool {
	return a.Index.Cmp(b.Index) < 0
}

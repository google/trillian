package merkle

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"sort"
	"testing"

	"github.com/google/trillian"
)

// This root was calculated with the C++/Python sparse merkle tree code in the
// github.com/google/certificate-transparency repo.
const sparseEmptyRootHashB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="

func mustDecode(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

type sparseRefValue struct {
	index *big.Int
	value []byte
}

func (s *sparseRefValue) String() string {
	return fmt.Sprintf("sparseRefValue{index: %b, value: %v", s.index, s.value)
}

type by func(a, b *sparseRefValue) bool

func (by by) Sort(values []sparseRefValue) {
	s := &valueSorter{
		values: values,
		by:     by,
	}
	sort.Sort(s)
}

type valueSorter struct {
	values []sparseRefValue
	by     func(a, b *sparseRefValue) bool
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

type sparseReference struct {
	hasher          TreeHasher
	hStarEmptyCache []trillian.Hash
}

func newSparseReference() sparseReference {
	h := NewTreeHasher(trillian.NewSHA256())
	return sparseReference{
		hasher:          h,
		hStarEmptyCache: []trillian.Hash{h.HashLeaf([]byte(""))},
	}
}

func indexLess(a, b *sparseRefValue) bool {
	return a.index.Cmp(b.index) < 0
}

func (s *sparseReference) HStar2(n int, values []sparseRefValue) (trillian.Hash, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.HStar2b(n, values, 0, len(values), offset)
}

func (s *sparseReference) HStarEmpty(n int) (trillian.Hash, error) {
	if len(s.hStarEmptyCache) <= n {
		emptyRoot, err := s.HStarEmpty(n - 1)
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

func (s *sparseReference) HStar2b(n int, values []sparseRefValue, lo, hi int, offset *big.Int) (trillian.Hash, error) {
	if n == 0 {
		if lo == hi {
			return s.hStarEmptyCache[0], nil
		}
		if hiLoDelta := hi - lo; hiLoDelta != 1 {
			return nil, fmt.Errorf("hi-lo is not 1, but %d", hiLoDelta)
		}
		return s.hasher.HashLeaf(values[lo].value), nil
	}
	if lo == hi {
		return s.HStarEmpty(n)
	}

	split := new(big.Int).Lsh(big.NewInt(1), uint(n-1))
	split.Add(split, offset)
	i := lo + sort.Search(hi-lo, func(i int) bool { return values[lo+i].index.Cmp(split) >= 0 })
	lhs, err := s.HStar2b(n-1, values, lo, i, offset)
	if err != nil {
		return nil, err
	}
	rhs, err := s.HStar2b(n-1, values, i, hi, split)
	if err != nil {
		return nil, err
	}
	return s.hasher.HashChildren(lhs, rhs), nil
}

// createSparseRefValues builds a list of sparseRefValue structs suitable for
// passing into a the HStar2 sparse reference implementation.
// The map keys will be SHA256 hashed before being added to the returned
// structs.
func createSparseRefValues(vs map[string]string) []sparseRefValue {
	r := []sparseRefValue{}
	for k := range vs {
		h := sha256.Sum256([]byte(k))
		r = append(r, sparseRefValue{
			index: new(big.Int).SetBytes(h[:]),
			value: []byte(vs[k]),
		})
	}
	return r
}

func TestReferenceEmptyRootKAT(t *testing.T) {
	s := newSparseReference()
	root, err := s.HStar2(s.hasher.Size()*8, []sparseRefValue{})
	if err != nil {
		t.Fatalf("Failed to calculate root: %v", err)
	}
	if expected, got := mustDecode(sparseEmptyRootHashB64), root; !bytes.Equal(expected, got) {
		t.Fatalf("Expected empty root:\n%v\nGot:\n%v", expected, got)
	}
}

func TestReferenceSimpleDataSetKAT(t *testing.T) {
	s := newSparseReference()
	vector := []struct{ k, v, rootB64 string }{
		{"a", "0", "nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0="},
		{"b", "1", "EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY="},
		{"a", "2", "2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o="},
	}

	m := make(map[string]string)
	for i, x := range vector {
		m[x.k] = x.v
		values := createSparseRefValues(m)
		root, err := s.HStar2(s.hasher.Size()*8, values)
		if err != nil {
			t.Fatalf("Failed to calculate root at iteration %d: %v", err, i)
		}
		if expected, got := mustDecode(x.rootB64), root; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root:\n%v\nGot:\n%v", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}
}

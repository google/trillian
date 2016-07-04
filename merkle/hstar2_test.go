package merkle

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"testing"

	"github.com/google/trillian"
)

// This root was calculated with the C++/Python sparse merkle tree code in the
// github.com/google/certificate-transparency repo.
const sparseEmptyRootHashB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="

// TODO(al): collect these test helpers together somewhere.
func mustDecode(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

// createHStar2Leaves builds a list of HStar2LeafHash structs suitable for
// passing into a the HStar2 sparse merkle tree implementation.
// The map keys will be SHA256 hashed before being added to the returned
// structs.
func createHStar2Leaves(th TreeHasher, values map[string]string) []HStar2LeafHash {
	r := []HStar2LeafHash{}
	for k := range values {
		khash := sha256.Sum256([]byte(k))
		vhash := th.HashLeaf([]byte(values[k]))
		r = append(r, HStar2LeafHash{
			Index:    new(big.Int).SetBytes(khash[:]),
			LeafHash: vhash[:],
		})
	}
	return r
}

func TestHStar2EmptyRootKAT(t *testing.T) {
	th := NewRFC6962TreeHasher(trillian.NewSHA256())
	s := NewHStar2(th)
	root, err := s.HStar2Root(s.hasher.Size()*8, []HStar2LeafHash{})
	if err != nil {
		t.Fatalf("Failed to calculate root: %v", err)
	}
	if expected, got := mustDecode(sparseEmptyRootHashB64), root; !bytes.Equal(expected, got) {
		t.Fatalf("Expected empty root:\n%v\nGot:\n%v", expected, got)
	}
}

// Some known answers for incrementally adding key/value pairs to a sparse tree.
// rootB64 is the incremental root after adding the corresponding k/v pair, and
// all k/v pairs which come before it.
var simpleTestVector = []struct{ k, v, rootB64 string }{
	{"a", "0", "nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0="},
	{"b", "1", "EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY="},
	{"a", "2", "2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o="},
}

func TestHStar2SimpleDataSetKAT(t *testing.T) {
	th := NewRFC6962TreeHasher(trillian.NewSHA256())
	s := NewHStar2(th)

	m := make(map[string]string)
	for i, x := range simpleTestVector {
		m[x.k] = x.v
		values := createHStar2Leaves(th, m)
		root, err := s.HStar2Root(s.hasher.Size()*8, values)
		if err != nil {
			t.Fatalf("Failed to calculate root at iteration %d: %v", i, err)
		}
		if expected, got := mustDecode(x.rootB64), root; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root:\n%v\nGot:\n%v", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}
}

// TestHStar2GetSet ensures that we get the same roots as above when we
// incrementally calculate roots.
func TestHStar2GetSet(t *testing.T) {
	th := NewRFC6962TreeHasher(trillian.NewSHA256())

	// Node cache is shared between tree builds and in effect plays the role of
	// the TreeStorage layer.
	cache := make(map[string]trillian.Hash)

	for i, x := range simpleTestVector {
		s := NewHStar2(th)
		m := make(map[string]string)
		m[x.k] = x.v
		values := createHStar2Leaves(th, m)
		// ensure we're going incrementally, one leaf at a time.
		if len(values) != 1 {
			t.Fatalf("Should only have 1 leaf per run, got %d", len(values))
		}
		root, err := s.HStar2Nodes(s.hasher.Size()*8, values,
			func(depth int, index *big.Int) (trillian.Hash, error) {
				return cache[fmt.Sprintf("%x/%d", index, depth)], nil
			},
			func(depth int, index *big.Int, hash trillian.Hash) error {
				cache[fmt.Sprintf("%x/%d", index, depth)] = hash
				return nil
			})
		if err != nil {
			t.Fatalf("Failed to calculate root at iteration %d: %v", i, err)
		}
		if expected, got := mustDecode(x.rootB64), root; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root:\n%v\nGot:\n%v", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}
}

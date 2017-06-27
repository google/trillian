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
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/testonly"
)

const treeID = int64(0)

// This root was calculated with the C++/Python sparse Merkle tree code in the
// github.com/google/certificate-transparency repo.
// TODO(alcutter): replace with hash-dependent computation. How is this computed?
var sparseEmptyRootHashB64 = testonly.MustDecodeBase64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")

// createHStar2Leaves builds a list of HStar2LeafHash structs suitable for
// passing into a the HStar2 sparse Merkle tree implementation.
// The map keys will be SHA256 hashed before being added to the returned
// structs.
func createHStar2Leaves(treeID int64, hasher hashers.MapHasher, values map[string]string) []HStar2LeafHash {
	r := []HStar2LeafHash{}
	for k := range values {
		khash := testonly.HashKey(k)
		vhash := hasher.HashLeaf(treeID, khash, hasher.BitLen(), []byte(values[k]))
		r = append(r, HStar2LeafHash{
			Index:    new(big.Int).SetBytes(khash),
			LeafHash: vhash[:],
		})
	}
	return r
}

func TestHStar2EmptyRootKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)
	root, err := s.HStar2Root(s.hasher.Size()*8, []HStar2LeafHash{})
	if err != nil {
		t.Fatalf("Failed to calculate root: %v", err)
	}
	if got, want := root, sparseEmptyRootHashB64; !bytes.Equal(got, want) {
		t.Fatalf("Expected empty root. Got: \n%x\nWant:\n%x", got, want)
	}
}

// Some known answers for incrementally adding key/value pairs to a sparse tree.
// rootB64 is the incremental root after adding the corresponding k/v pair, and
// all k/v pairs which come before it.
var simpleTestVector = []struct {
	k, v string
	root []byte
}{
	{"a", "0", testonly.MustDecodeBase64("nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0=")},
	{"b", "1", testonly.MustDecodeBase64("EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY=")},
	{"a", "2", testonly.MustDecodeBase64("2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o=")},
}

func TestHStar2SimpleDataSetKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	m := make(map[string]string)
	for i, x := range simpleTestVector {
		m[x.k] = x.v
		values := createHStar2Leaves(treeID, maphasher.Default, m)
		root, err := s.HStar2Root(s.hasher.Size()*8, values)
		if err != nil {
			t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
			continue
		}
		if got, want := root, x.root; !bytes.Equal(got, want) {
			t.Errorf("Root: \n%x, want:\n%x", got, want)
		}
	}
}

// TestHStar2GetSet ensures that we get the same roots as above when we
// incrementally calculate roots.
func TestHStar2GetSet(t *testing.T) {
	// Node cache is shared between tree builds and in effect plays the role of
	// the TreeStorage layer.
	cache := make(map[string][]byte)
	hasher := maphasher.Default

	for i, x := range simpleTestVector {
		s := NewHStar2(treeID, hasher)
		m := make(map[string]string)
		m[x.k] = x.v
		values := createHStar2Leaves(treeID, hasher, m)
		// ensure we're going incrementally, one leaf at a time.
		if len(values) != 1 {
			t.Fatalf("Should only have 1 leaf per run, got %d", len(values))
		}
		root, err := s.HStar2Nodes(s.hasher.BitLen(), 0, values,
			func(depth int, index *big.Int) ([]byte, error) {
				return cache[fmt.Sprintf("%x/%d", index, depth)], nil
			},
			func(depth int, index *big.Int, hash []byte) error {
				cache[fmt.Sprintf("%x/%d", index, depth)] = hash
				return nil
			})
		if err != nil {
			t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
			continue
		}
		if got, want := root, x.root; !bytes.Equal(got, want) {
			t.Errorf("Root:\n%x, want:\n%x", got, want)
		}
	}
}

// Checks that we calculate the same empty root hash as a 256-level tree has
// when calculating top subtrees using an appropriate offset.
func TestHStar2OffsetEmptyRootKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	for size := 1; size < 255; size++ {
		root, err := s.HStar2Nodes(size, s.hasher.Size()*8-size, []HStar2LeafHash{},
			func(int, *big.Int) ([]byte, error) { return nil, nil },
			func(int, *big.Int, []byte) error { return nil })
		if err != nil {
			t.Fatalf("Failed to calculate root %v", err)
		}
		if got, want := root, sparseEmptyRootHashB64; !bytes.Equal(got, want) {
			t.Fatalf("Got root:\n%x, want:\n%x", got, want)
		}
	}
}

// Create intermediate "root" values for the passed in HStar2LeafHashes.
// These "root" hashes are from (assumed distinct) subtrees of size
// 256-prefixSize, and can be passed in as leaves to top-subtree calculation.
func rootsForTrimmedKeys(t *testing.T, prefixSize int, lh []HStar2LeafHash) []HStar2LeafHash {
	var ret []HStar2LeafHash
	s := NewHStar2(treeID, maphasher.Default)
	for i := range lh {
		prefix := new(big.Int).Rsh(lh[i].Index, uint(s.hasher.Size()*8-prefixSize))
		b := lh[i].Index.Bytes()
		// ensure we've got any chopped of leading zero bytes
		for len(b) < 32 {
			b = append([]byte{0}, b...)
		}
		lh[i].Index.SetBytes(b[prefixSize/8:])
		root, err := s.HStar2Root(s.hasher.Size()*8-prefixSize, []HStar2LeafHash{lh[i]})
		if err != nil {
			t.Fatalf("Failed to calculate root %v", err)
		}
		ret = append(ret, HStar2LeafHash{prefix, root})
	}
	return ret
}

// Checks that splitting the calculation of a 256-level tree into two phases
// (single top subtree of size n, and multipl bottom subtrees of size 256-n)
// still arrives at the same Known Answers for root hash.
func TestHStar2OffsetRootKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	m := make(map[string]string)

	for i, x := range simpleTestVector {
		// start at 24 so we can assume that key prefixes are probably unique by then
		// TODO(al): improve rootsForTrimmedKeys to use a map and remove this
		// requirement.
		for size := 24; size < 256; size += 8 {
			m[x.k] = x.v
			intermediates := rootsForTrimmedKeys(t, size, createHStar2Leaves(treeID, maphasher.Default, m))

			root, err := s.HStar2Nodes(size, s.hasher.Size()*8-size, intermediates,
				func(int, *big.Int) ([]byte, error) { return nil, nil },
				func(int, *big.Int, []byte) error { return nil })
			if err != nil {
				t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
				continue
			}
			if got, want := root, x.root; !bytes.Equal(got, want) {
				t.Errorf("Root:\n%x\n, want:\n%x", got, want)
			}
		}
	}
}

func TestHStar2NegativeTreeLevelOffset(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	_, err := s.HStar2Nodes(32, -1, []HStar2LeafHash{},
		func(int, *big.Int) ([]byte, error) { return nil, nil },
		func(int, *big.Int, []byte) error { return nil })
	if got, want := err, ErrNegativeTreeLevelOffset; got != want {
		t.Fatalf("Hstar2Nodes(): %v, want %v", got, want)
	}
}

func TestPaddedBytes(t *testing.T) {
	size := 160 / 8
	for _, tc := range []struct {
		i    *big.Int
		want []byte
	}{
		{i: big.NewInt(0), want: h2b("0000000000000000000000000000000000000000")},
		{i: big.NewInt(1), want: h2b("0000000000000000000000000000000000000001")},
		{i: new(big.Int).SetBytes(h2b("00FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0F")), want: h2b("00FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0F")},
		{i: new(big.Int).SetBytes(h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0F")), want: h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF0F")},
	} {
		if got, want := PaddedBytes(tc.i, size), tc.want; !bytes.Equal(got, want) {
			t.Errorf("PaddedBytes(%d): %x, want %x", tc.i, got, want)
		}
	}
}
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

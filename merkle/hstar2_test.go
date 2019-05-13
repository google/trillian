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
	"crypto"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
)

const treeID = int64(0)

var (
	deB64 = testonly.MustDecodeBase64
)

// Some known answers for incrementally adding index/value pairs to a sparse tree.
// rootB64 is the incremental root after adding the corresponding i/v pair, and
// all i/v pairs which come before it.
//
// Values were calculated with the C++/Python sparse Merkle tree code in the
// github.com/google/certificate-transparency repo.
// TODO(alcutter): replace with hash-dependent computation. How is this computed?
var simpleTestVector = []struct {
	index, value, root []byte
}{
	{nil, nil, deB64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")}, // Empty tree.
	{testonly.HashKey("a"), []byte("0"), deB64("nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0=")},
	{testonly.HashKey("b"), []byte("1"), deB64("EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY=")},
	{testonly.HashKey("a"), []byte("2"), deB64("2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o=")},
}

// createHStar2Leaves returns a []HStar2LeafHash formed by the mapping of index, value ...
// createHStar2Leaves panics if len(iv) is odd. Duplicate i/v pairs get over written.
func createHStar2Leaves(treeID int64, hasher hashers.MapHasher, iv ...[]byte) ([]*HStar2LeafHash, error) {
	if len(iv)%2 != 0 {
		panic(fmt.Sprintf("merkle: createHstar2Leaves got odd number of iv pairs: %v", len(iv)))
	}
	m := make(map[string]*HStar2LeafHash)
	var index []byte
	for i, b := range iv {
		if i%2 == 0 {
			index = b
			continue
		}
		leafHash, err := hasher.HashLeaf(treeID, index, b)
		if err != nil {
			return nil, err
		}
		m[fmt.Sprintf("%x", index)] = &HStar2LeafHash{
			Index:    new(big.Int).SetBytes(index),
			LeafHash: leafHash,
		}
	}

	r := make([]*HStar2LeafHash, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r, nil
}

func TestHStar2SimpleDataSetKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	iv := [][]byte{}
	for i, x := range simpleTestVector {
		iv = append(iv, x.index, x.value)
		values, err := createHStar2Leaves(treeID, maphasher.Default, iv...)
		if err != nil {
			t.Fatalf("createHStar2Leaves(): %v", err)
		}
		root, err := s.HStar2Root(s.hasher.BitLen(), values)
		if err != nil {
			t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
			continue
		}
		if got, want := root, x.root; !bytes.Equal(got, want) {
			t.Errorf("Root: %x, want: %x", got, want)
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
		values, err := createHStar2Leaves(treeID, hasher, x.index, x.value)
		if err != nil {
			t.Fatalf("createHStar2Leaves(): %v", err)
		}
		// ensure we're going incrementally, one leaf at a time.
		if len(values) != 1 {
			t.Fatalf("Should only have 1 leaf per run, got %d", len(values))
		}
		root, err := s.HStar2Nodes(nil, s.hasher.BitLen(), values,
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
			t.Errorf("Root: %x, want: %x", got, want)
		}
	}
}

// Create intermediate "root" values for the passed in HStar2LeafHashes.
// These "root" hashes are from (assumed distinct) subtrees of size
// 256-prefixSize, and can be passed in as leaves to top-subtree calculation.
func rootsForTrimmedKeys(t *testing.T, prefixSize int, lh []*HStar2LeafHash) []*HStar2LeafHash {
	var ret []*HStar2LeafHash
	hasher := maphasher.Default
	s := NewHStar2(treeID, hasher)
	for i := range lh {
		subtreeDepth := s.hasher.BitLen() - prefixSize
		prefix := lh[i].Index.Bytes()
		// Left pad prefix with zeros back out to 32 bytes.
		for len(prefix) < 32 {
			prefix = append([]byte{0}, prefix...)
		}
		prefix = prefix[:prefixSize/8] // We only want the first prefixSize bytes.
		root, err := s.HStar2Nodes(prefix, subtreeDepth, []*HStar2LeafHash{lh[i]}, nil, nil)
		if err != nil {
			t.Fatalf("Failed to calculate root %v", err)
		}

		ret = append(ret, &HStar2LeafHash{
			Index:    storage.NewNodeIDFromPrefixSuffix(prefix, storage.EmptySuffix, hasher.BitLen()).BigInt(),
			LeafHash: root,
		})
	}
	return ret
}

// Checks that splitting the calculation of a 256-level tree into two phases
// (single top subtree of size n, and multipl bottom subtrees of size 256-n)
// still arrives at the same Known Answers for root hash.
func TestHStar2OffsetRootKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)
	iv := [][]byte{}
	for i, x := range simpleTestVector {
		iv = append(iv, x.index, x.value)
		// start at 24 so we can assume that key prefixes are probably unique by then
		// TODO(al): improve rootsForTrimmedKeys to use a map and remove this
		// requirement.
		for size := 24; size < 256; size += 8 {
			leaves, err := createHStar2Leaves(treeID, maphasher.Default, iv...)
			if err != nil {
				t.Fatalf("createHStar2Leaves(): %v", err)
			}
			intermediates := rootsForTrimmedKeys(t, size, leaves)

			root, err := s.HStar2Nodes(nil, size, intermediates, nil, nil)
			if err != nil {
				t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
				continue
			}
			if got, want := root, x.root; !bytes.Equal(got, want) {
				t.Errorf("HStar2Nodes(i: %v, size:%v): %x, want: %x", i, size, got, want)
			}
		}
	}
}

func TestHStar2NegativeTreeLevelOffset(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	_, err := s.HStar2Nodes(make([]byte, 31), 9, []*HStar2LeafHash{}, nil, nil)
	if got, want := err, ErrSubtreeOverrun; got != want {
		t.Fatalf("Hstar2Nodes(): %v, want %v", got, want)
	}
}

func BenchmarkHStar2Root(b *testing.B) {
	hs2 := NewHStar2(42, coniks.New(crypto.SHA256))
	for i := 0; i < b.N; i++ {
		_, err := hs2.HStar2Root(256, leafHashes(b, 200))
		if err != nil {
			b.Fatalf("hstar2 root failed: %v", err)
		}
	}
}

func leafHashes(b *testing.B, n int) []*HStar2LeafHash {
	b.Helper()
	// Use a fixed sequence to ensure runs are comparable
	r := rand.New(rand.NewSource(42424242))
	lh := make([]*HStar2LeafHash, 0, n)

	for l := 0; l < n; l++ {
		h := make([]byte, 32)
		if _, err := r.Read(h); err != nil {
			b.Fatalf("Failed to make random leaf hashes: %v", err)
		}
		lh = append(lh, &HStar2LeafHash{
			LeafHash: h,
			Index:    big.NewInt(r.Int63()),
		})
	}

	return lh
}

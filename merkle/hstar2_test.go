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
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/testonly"
)

const treeID = int64(0)

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
	{nil, nil, b64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")}, // Empty tree.
	{testonly.HashKey("a"), []byte("0"), b64("nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0=")},
	{testonly.HashKey("b"), []byte("1"), b64("EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY=")},
	{testonly.HashKey("a"), []byte("2"), b64("2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o=")},
}

// createHStar2Leaves returns a []HStar2LeafHash formed by the mapping of index, value ...
// createHStar2Leaves panics if len(iv) is odd. Duplicate i/v pairs get over written.
func createHStar2Leaves(treeID int64, hasher hashers.MapHasher, iv ...[]byte) []HStar2LeafHash {
	if len(iv)%2 != 0 {
		panic(fmt.Sprintf("merkle: createHstar2Leaves got odd number of iv pairs: %v", len(iv)))
	}
	m := make(map[string]HStar2LeafHash)
	var index []byte
	for i, b := range iv {
		if i%2 == 0 {
			index = b
			continue
		}
		m[fmt.Sprintf("%x", index)] = HStar2LeafHash{
			Index:    new(big.Int).SetBytes(index),
			LeafHash: hasher.HashLeaf(treeID, index, hasher.BitLen(), b),
		}
	}

	r := make([]HStar2LeafHash, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r
}

func TestHStar2SimpleDataSetKAT(t *testing.T) {
	s := NewHStar2(treeID, maphasher.Default)

	iv := [][]byte{}
	for i, x := range simpleTestVector {
		iv = append(iv, x.index, x.value)
		values := createHStar2Leaves(treeID, maphasher.Default, iv...)
		root, err := s.HStar2Root(s.hasher.BitLen(), values)
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
		values := createHStar2Leaves(treeID, hasher, x.index, x.value)
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

// Create intermediate "root" values for the passed in HStar2LeafHashes.
// These "root" hashes are from (assumed distinct) subtrees of size
// 256-prefixSize, and can be passed in as leaves to top-subtree calculation.
func rootsForTrimmedKeys(t *testing.T, prefixSize int, lh []HStar2LeafHash) []HStar2LeafHash {
	var ret []HStar2LeafHash
	s := NewHStar2(treeID, maphasher.Default)
	for i := range lh {
		prefix := new(big.Int).Rsh(lh[i].Index, uint(s.hasher.BitLen()-prefixSize))
		b := lh[i].Index.Bytes()
		// ensure we've got any chopped of leading zero bytes
		for len(b) < 32 {
			b = append([]byte{0}, b...)
		}
		lh[i].Index.SetBytes(b[prefixSize/8:])
		root, err := s.HStar2Root(s.hasher.BitLen()-prefixSize, []HStar2LeafHash{lh[i]})
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
	iv := [][]byte{}
	for i, x := range simpleTestVector {
		iv = append(iv, x.index, x.value)
		// start at 24 so we can assume that key prefixes are probably unique by then
		// TODO(al): improve rootsForTrimmedKeys to use a map and remove this
		// requirement.
		for size := 24; size < 256; size += 8 {
			leaves := createHStar2Leaves(treeID, maphasher.Default, iv...)
			intermediates := rootsForTrimmedKeys(t, size, leaves)

			root, err := s.HStar2Nodes(size, s.hasher.BitLen()-size, intermediates,
				func(int, *big.Int) ([]byte, error) { return nil, nil },
				func(int, *big.Int, []byte) error { return nil })
			if err != nil {
				t.Errorf("Failed to calculate root at iteration %d: %v", i, err)
				continue
			}
			if got, want := root, x.root; !bytes.Equal(got, want) {
				t.Errorf("Root: %x, want: %x", got, want)
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

// b64 converts a base64 string into []byte.
func b64(b64 string) []byte {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic("invalid base64 string")
	}
	return b
}

// h2b converts a hex string into []byte.
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

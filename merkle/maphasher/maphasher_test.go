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
package maphasher

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"testing"

	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
)

const (
	// Expected root hash of an empty sparse Merkle tree.
	// This was taken from the C++ SparseMerkleTree tests in
	// github.com/google/certificate-transparency.
	emptyMapRootB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="
	treeID          = int64(0)
)

var h2b = testonly.MustHexDecode

func TestEmptyRoot(t *testing.T) {
	emptyRoot, err := base64.StdEncoding.DecodeString(emptyMapRootB64)
	if err != nil {
		t.Fatalf("couldn't decode empty root base64 constant.")
	}
	mh := New(crypto.SHA256)
	if got, want := mh.HashEmpty(treeID, tree.NodeID2{}), emptyRoot; !bytes.Equal(got, want) {
		t.Fatalf("HashEmpty(0): %x, want %x", got, want)
	}
}

func TestHashLeaf(t *testing.T) {
	tests := []struct {
		node  tree.NodeID2
		value []byte
		want  []byte
	}{
		{tree.NewNodeID2WithLast("", 0, 8), nil, h2b("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d")},
		{tree.NewNodeID2WithLast("", 0, 8), []byte(""), h2b("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d")},
		{tree.NewNodeID2WithLast("", 1, 8), []byte(""), h2b("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d")},
		{tree.NewNodeID2WithLast("", 1, 8), []byte("foo"), h2b("1d2039fa7971f4bf01a1c20cb2a3fe7af46865ca9cd9b840c2063df8fec4ff75")},
	}
	for _, test := range tests {
		got := Default.HashLeaf(6962, test.node, test.value)
		if !bytes.Equal(got, test.want) {
			t.Errorf("HashLeaf(%x)=%x; want %x", test.value, got, test.want)
		}
	}
}

// Compares the old HStar2 empty branch algorithm to the new.
func TestHStar2Equivalence(t *testing.T) {
	m := New(crypto.SHA256)
	leafHash := m.HashLeaf(treeID, tree.NodeID2{}, []byte(""))
	star := hstar{
		hasher:          m,
		hStarEmptyCache: [][]byte{leafHash},
	}
	zero := string(make([]byte, m.Size()))
	fullDepth := m.Size() * 8
	for i := 0; i <= fullDepth; i++ {
		if got, want := m.HashEmpty(treeID, tree.NewNodeID2(zero, uint(fullDepth-i))), star.hStarEmpty(i); !bytes.Equal(got, want) {
			t.Errorf("HashEmpty(%v): \n%x, want: \n%x", i, got, want)
		}
	}
}

// Old hstar2 empty cache algorithm.
type hstar struct {
	hasher          *MapHasher
	hStarEmptyCache [][]byte
}

// hStarEmpty calculates (and caches) the "null-hash" for the requested tree level.
// Note: here level 0 is the leaf and level 255 is the root.
func (s *hstar) hStarEmpty(n int) []byte {
	if len(s.hStarEmptyCache) <= n {
		emptyRoot := s.hStarEmpty(n - 1)
		h := s.hasher.HashChildren(emptyRoot, emptyRoot)
		s.hStarEmptyCache = append(s.hStarEmptyCache, h)
	}
	return s.hStarEmptyCache[n]
}

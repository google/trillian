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
package rfc6962

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/google/trillian/merkle"
)

const (
	// Expected root hash of an empty sparse Merkle tree.
	// This was taken from the C++ SparseMerkleTree tests in
	// github.com/google/certificate-transparency.
	emptyMapRootB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="
)

func TestEmptyRoot(t *testing.T) {
	emptyRoot, err := base64.StdEncoding.DecodeString(emptyMapRootB64)
	if err != nil {
		t.Fatalf("couldn't decode empty root base64 constant.")
	}
	mh := New(crypto.SHA256)
	rootLevel := mh.Size() * 8
	if got, want := mh.HashEmpty(rootLevel), emptyRoot; !bytes.Equal(got, want) {
		t.Fatalf("HashEmpty(0): %x, want %x", got, want)
	}
}

// Compares the old HStar2 empty branch algorithm to the new.
func TestHStar2Equivalence(t *testing.T) {
	m := New(crypto.SHA256)
	star := hstar{
		hasher:          m,
		hStarEmptyCache: [][]byte{m.HashLeaf([]byte(""))},
	}
	fullDepth := m.Size() * 8
	for i := 0; i < fullDepth; i++ {
		if got, want := m.HashEmpty(i), star.hStarEmpty(i); !bytes.Equal(got, want) {
			t.Errorf("HashEmpty(%v): \n%x, want: \n%x", i, got, want)
		}
	}
}

// Old hstar2 empty cache algorithm.
type hstar struct {
	hasher          merkle.MapHasher
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

func TestRfc6962Hasher(t *testing.T) {
	hasher := DefaultHasher

	for _, tc := range []struct {
		desc string
		got  []byte
		want string
	}{
		// echo -n | sha256sum
		{desc: "RFC962 Empty", want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", got: hasher.EmptyRoot()},
		// echo -n 004C313233343536 | xxd -r -p | sha256sum
		{desc: "RFC6962 Leaf", want: "395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56", got: hasher.HashLeaf([]byte("L123456"))},
		// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
		{desc: "RFC6962 Node", want: "aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb", got: hasher.HashChildren([]byte("N123"), []byte("N456"))},
	} {
		wantBytes, err := hex.DecodeString(tc.want)
		if err != nil {
			t.Errorf("hex.DecodeString(%v): %v", tc.want, err)
			continue
		}

		if got, want := tc.got, wantBytes; !bytes.Equal(got, want) {
			t.Fatalf("%v: got %x, want %x", tc.desc, got, want)
		}
	}
}

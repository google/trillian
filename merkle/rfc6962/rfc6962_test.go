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
	"encoding/hex"
	"fmt"
	"testing"

	_ "github.com/golang/glog"
)

func TestRFC6962Hasher(t *testing.T) {
	hasher := DefaultHasher

	leafHash := hasher.HashLeaf([]byte("L123456"))
	emptyLeafHash := hasher.HashLeaf([]byte{})

	for _, tc := range []struct {
		desc string
		got  []byte
		want string
	}{
		// echo -n | sha256sum
		{
			desc: "RFC6962 Empty",
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			got:  hasher.EmptyRoot(),
		},
		// Check that the empty hash is not the same as the hash of an empty leaf.
		// echo -n 00 | xxd -r -p | sha256sum
		{
			desc: "RFC6962 Empty Leaf",
			want: "6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
			got:  emptyLeafHash,
		},
		// echo -n 004C313233343536 | xxd -r -p | sha256sum
		{
			desc: "RFC6962 Leaf",
			want: "395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56",
			got:  leafHash,
		},
		// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
		{
			desc: "RFC6962 Node",
			want: "aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb",
			got:  hasher.HashChildren([]byte("N123"), []byte("N456")),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			wantBytes, err := hex.DecodeString(tc.want)
			if err != nil {
				t.Fatalf("hex.DecodeString(%x): %v", tc.want, err)
			}
			if got, want := tc.got, wantBytes; !bytes.Equal(got, want) {
				t.Errorf("got %x, want %x", got, want)
			}
		})
	}
}

// TODO(pavelkalinnikov): Apply this test to all LogHasher implementations.
func TestRFC6962HasherCollisions(t *testing.T) {
	hasher := DefaultHasher

	// Check that different leaves have different hashes.
	leaf1, leaf2 := []byte("Hello"), []byte("World")
	hash1 := hasher.HashLeaf(leaf1)
	hash2 := hasher.HashLeaf(leaf2)
	if bytes.Equal(hash1, hash2) {
		t.Errorf("Leaf hashes should differ, but both are %x", hash1)
	}

	// Compute an intermediate subtree hash.
	subHash1 := hasher.HashChildren(hash1, hash2)
	// Check that this is not the same as a leaf hash of their concatenation.
	preimage := append(hash1, hash2...)
	forgedHash := hasher.HashLeaf(preimage)
	if bytes.Equal(subHash1, forgedHash) {
		t.Errorf("Hasher is not second-preimage resistant")
	}

	// Swap the order of nodes and check that the hash is different.
	subHash2 := hasher.HashChildren(hash2, hash1)
	if bytes.Equal(subHash1, subHash2) {
		t.Errorf("Subtree hash does not depend on the order of leaves")
	}
}

// TODO(al): Remove me.
func BenchmarkHashChildrenOld(b *testing.B) {
	h := DefaultHasher
	l := h.HashLeaf([]byte("one"))
	r := h.HashLeaf([]byte("or other"))
	for i := 0; i < b.N; i++ {
		_ = h.hashChildrenOld(l, r)
	}
}

func BenchmarkHashChildren(b *testing.B) {
	h := DefaultHasher
	l := h.HashLeaf([]byte("one"))
	r := h.HashLeaf([]byte("or other"))
	for i := 0; i < b.N; i++ {
		_ = h.HashChildren(l, r)
	}
}

func TestHashChildrenEquivToOld(t *testing.T) {
	h := DefaultHasher
	for i := 0; i < 1000; i++ {
		l := h.HashLeaf([]byte(fmt.Sprintf("leaf left %d", i)))
		r := h.HashLeaf([]byte(fmt.Sprintf("leaf right %d", i)))
		if oldHash, newHash := h.hashChildrenOld(l, r), h.HashChildren(l, r); !bytes.Equal(oldHash, newHash) {
			t.Errorf("%d different hashes: %x vs %x", i, oldHash, newHash)
		}
	}

}

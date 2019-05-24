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

package compact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
)

func mustGetRoot(t *testing.T, mt *Tree) []byte {
	t.Helper()
	hash, err := mt.CurrentRoot()
	if err != nil {
		t.Fatalf("CurrentRoot: %v", err)
	}
	return hash
}

// This returns something that won't result in a valid root hash match, doesn't really
// matter what it is but it must be correct length for an SHA256 hash as if it was real
func fixedHashGetNodesFunc(ids []NodeID) [][]byte {
	hashes := make([][]byte, len(ids))
	for i := range ids {
		hashes[i] = []byte("12345678901234567890123456789012")
	}
	return hashes
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	hashes := fixedHashGetNodesFunc(TreeNodes(237))

	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewTreeWithState(rfc6962.DefaultHasher, 237, hashes, []byte("nomatch!nomatch!nomatch!nomatch!"))
	if err == nil || !strings.HasPrefix(err.Error(), "root hash mismatch") {
		t.Errorf("Did not return correct error on root mismatch: %v", err)
	}
}

func TestCompactVsFullTree(t *testing.T) {
	imt := merkle.NewInMemoryMerkleTree(rfc6962.DefaultHasher)
	nodes := make(map[NodeID][]byte)

	getHashes := func(ids []NodeID) [][]byte {
		hashes := make([][]byte, len(ids))
		for i, id := range ids {
			hashes[i] = nodes[id]
		}
		return hashes
	}

	for i := uint64(0); i < 1024; i++ {
		hashes := getHashes(TreeNodes(i))
		cmt, err := NewTreeWithState(rfc6962.DefaultHasher, i, hashes, imt.CurrentRoot().Hash())
		if err != nil {
			t.Errorf("interation %d: failed to create CMT with state: %v", i, err)
		}
		if a, b := imt.CurrentRoot().Hash(), mustGetRoot(t, cmt); !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}

		newLeaf := []byte(fmt.Sprintf("Leaf %d", i))

		iSeq, iHash := imt.AddLeaf(newLeaf)
		cHash, err := cmt.AppendLeaf(newLeaf, func(id NodeID, hash []byte) {
			nodes[id] = hash
		})
		if err != nil {
			t.Fatalf("mt update failed: %v", err)
		}
		cSeq := cmt.Size() - 1 // The index of the last inserted leaf.

		// In-Memory tree is 1-based for sequence numbers, since it's based on the original CT C++ impl.
		if got, want := uint64(iSeq), i+1; got != want {
			t.Errorf("iteration %d: Got in-memory sequence number of %d, expected %d", i, got, want)
		}
		if uint64(iSeq) != cSeq+1 {
			t.Errorf("iteration %d: Got in-memory sequence number of %d but %d (zero based) from compact tree", i, iSeq, cSeq)
		}
		if a, b := iHash.Hash(), cHash; !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got leaf hash %v from in-memory tree, but %v from compact tree", i, a, b)
		}
		if a, b := imt.CurrentRoot().Hash(), mustGetRoot(t, cmt); !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}
	}

	// Build another compact Merkle tree by incrementally adding the leaves to an empty tree.
	cmt := NewTree(rfc6962.DefaultHasher)
	for i := int64(0); i < imt.LeafCount(); i++ {
		newLeaf := []byte(fmt.Sprintf("Leaf %d", i))
		_, err := cmt.AppendLeaf(newLeaf, nil)
		if err != nil {
			t.Fatalf("AppendLeaf(%d)=_,_,%v, want _,_,nil", i, err)
		}
		if got, want := cmt.Size(), uint64(i+1); got != want {
			t.Fatalf("new tree size=%d, want %d", got, want)
		}
	}
	if a, b := imt.CurrentRoot().Hash(), mustGetRoot(t, cmt); !bytes.Equal(a, b) {
		t.Errorf("got in-memory root of %v, but compact tree has root %v", a, b)
	}
}

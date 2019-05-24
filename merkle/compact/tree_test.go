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
	"math/bits"
	"strings"
	"testing"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	to "github.com/google/trillian/merkle/testonly"
	"github.com/kylelemons/godebug/pretty"
)

// checkSizeInvariant ensures that the compact Merkle tree has the right number
// of non-empty node hashes.
func checkSizeInvariant(t *Tree) error {
	size := t.Size()
	hashes := t.hashes()
	if got, want := len(hashes), bits.OnesCount64(size); got != want {
		return fmt.Errorf("hashes mismatch: have %v hashes, want %v", got, want)
	}
	for i, hash := range hashes {
		if len(hash) == 0 {
			return fmt.Errorf("missing node hash at index %d", i)
		}
	}
	return nil
}

func mustGetRoot(t *testing.T, mt *Tree) []byte {
	t.Helper()
	hash, err := mt.CurrentRoot()
	if err != nil {
		t.Fatalf("CurrentRoot: %v", err)
	}
	return hash
}

func TestAddingLeaves(t *testing.T) {
	inputs := to.LeafInputs()
	roots := to.RootHashes()
	hashes := to.CompactTrees()

	// Test the "same" thing in different ways, to ensure than any lazy update
	// strategy being employed by the implementation doesn't affect the
	// API-visible calculation of root & size.
	for _, tc := range []struct {
		desc   string
		breaks []int
	}{
		{desc: "one-by-one", breaks: []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{desc: "one-by-one-no-zero", breaks: []int{1, 2, 3, 4, 5, 6, 7, 8}},
		{desc: "all-at-once", breaks: []int{8}},
		{desc: "all-at-once-zero", breaks: []int{0, 8}},
		{desc: "two-chunks", breaks: []int{3, 8}},
		{desc: "two-chunks-zero", breaks: []int{0, 3, 8}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			tree := NewTree(rfc6962.DefaultHasher)
			idx := 0
			for _, br := range tc.breaks {
				for ; idx < br; idx++ {
					if _, err := tree.AppendLeaf(inputs[idx], nil); err != nil {
						t.Fatalf("AppendLeaf: %v", err)
					}
					if err := checkSizeInvariant(tree); err != nil {
						t.Fatalf("SizeInvariant check failed: %v", err)
					}
				}
				if got, want := tree.Size(), uint64(br); got != want {
					t.Errorf("Size()=%d, want %d", got, want)
				}
				if br > 0 {
					if got, want := mustGetRoot(t, tree), roots[br]; !bytes.Equal(got, want) {
						t.Errorf("root=%v, want %v", got, want)
					}
					if diff := pretty.Compare(tree.hashes(), hashes[br]); diff != "" {
						t.Errorf("post-hashes() diff:\n%v", diff)
					}
				} else {
					if got, want := mustGetRoot(t, tree), to.EmptyRootHash(); !bytes.Equal(got, want) {
						t.Errorf("root=%x, want %x (empty)", got, want)
					}
				}
			}
		})
	}
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

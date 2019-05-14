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
	"encoding/base64"
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"testing"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/testonly"
	"github.com/kylelemons/godebug/pretty"
)

// This check ensures that the compact Merkle tree contains the correct set of
// nodes, i.e. the node on level i is present iff i-th bit of tree size is 1.
func checkUnusedNodesInvariant(t *Tree) error {
	size := t.size
	sizeBits := bits.Len64(uint64(size))
	if got, want := len(t.nodes), sizeBits; got != want {
		return fmt.Errorf("nodes mismatch: have %v nodes, want %v", got, want)
	}
	for level := 0; level < sizeBits; level++ {
		if size&1 == 1 {
			if t.nodes[level] == nil {
				return fmt.Errorf("missing node at level %d", level)
			}
		} else if t.nodes[level] != nil {
			return fmt.Errorf("unexpected node at level %d", level)
		}
		size >>= 1
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
	inputs := testonly.MerkleTreeLeafTestInputs()
	roots := testonly.MerkleTreeLeafTestRootHashes()
	hashes := testonly.CompactMerkleTreeLeafTestNodeHashes()

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
					if _, err := tree.AddLeaf(inputs[idx], nil); err != nil {
						t.Fatalf("AddLeaf: %v", err)
					}
					if err := checkUnusedNodesInvariant(tree); err != nil {
						t.Fatalf("UnusedNodesInvariant check failed: %v", err)
					}
				}
				if got, want := tree.Size(), int64(br); got != want {
					t.Errorf("Size()=%d, want %d", got, want)
				}
				if br > 0 {
					if got, want := mustGetRoot(t, tree), roots[br-1]; !bytes.Equal(got, want) {
						t.Errorf("root=%v, want %v", got, want)
					}
					if diff := pretty.Compare(tree.Hashes(), hashes[br-1]); diff != "" {
						t.Errorf("post-Hashes() diff:\n%v", diff)
					}
				} else {
					if got, want := mustGetRoot(t, tree), testonly.EmptyMerkleTreeRootHash(); !bytes.Equal(got, want) {
						t.Errorf("root=%x, want %x (empty)", got, want)
					}
				}
			}
		})
	}
}

func failingGetNodesFunc(_ []NodeID) ([][]byte, error) {
	return nil, errors.New("bang")
}

// This returns something that won't result in a valid root hash match, doesn't really
// matter what it is but it must be correct length for an SHA256 hash as if it was real
func fixedHashGetNodesFunc(ids []NodeID) ([][]byte, error) {
	hashes := make([][]byte, len(ids))
	for i := range ids {
		hashes[i] = []byte("12345678901234567890123456789012")
	}
	return hashes, nil
}

func TestLoadingTreeFailsNodeFetch(t *testing.T) {
	_, err := NewTreeWithState(rfc6962.DefaultHasher, 237, failingGetNodesFunc, []byte("notimportant"))

	if err == nil || !strings.Contains(err.Error(), "bang") {
		t.Errorf("Did not return correctly on failed node fetch: %v", err)
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewTreeWithState(rfc6962.DefaultHasher, 237, fixedHashGetNodesFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
	if err == nil || !strings.HasPrefix(err.Error(), "root hash mismatch") {
		t.Errorf("Did not return correct error on root mismatch: %v", err)
	}
}

func TestCompactVsFullTree(t *testing.T) {
	imt := merkle.NewInMemoryMerkleTree(rfc6962.DefaultHasher)
	nodes := make(map[NodeID][]byte)

	getNode := func(id NodeID) ([]byte, error) {
		return nodes[id], nil
	}

	for i := int64(0); i < 1024; i++ {
		cmt, err := NewTreeWithState(
			rfc6962.DefaultHasher,
			imt.LeafCount(),
			func(ids []NodeID) ([][]byte, error) {
				hashes := make([][]byte, len(ids))
				for i, id := range ids {
					var err error
					hashes[i], err = getNode(id)
					if err != nil {
						return nil, err
					}
				}
				return hashes, nil
			}, imt.CurrentRoot().Hash())

		if err != nil {
			t.Errorf("interation %d: failed to create CMT with state: %v", i, err)
		}
		if a, b := imt.CurrentRoot().Hash(), mustGetRoot(t, cmt); !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}

		newLeaf := []byte(fmt.Sprintf("Leaf %d", i))

		iSeq, iHash, err := imt.AddLeaf(newLeaf)
		if err != nil {
			t.Errorf("AddLeaf(): %v", err)
		}

		cHash, err := cmt.AddLeaf(newLeaf, func(id NodeID, hash []byte) {
			nodes[id] = hash
		})
		if err != nil {
			t.Fatalf("mt update failed: %v", err)
		}
		cSeq := cmt.Size() - 1 // The index of the last inserted leaf.

		// In-Memory tree is 1-based for sequence numbers, since it's based on the original CT C++ impl.
		if got, want := iSeq, i+1; got != want {
			t.Errorf("iteration %d: Got in-memory sequence number of %d, expected %d", i, got, want)
		}
		if int64(iSeq) != cSeq+1 {
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
		_, err := cmt.AddLeaf(newLeaf, nil)
		if err != nil {
			t.Fatalf("AddLeaf(%d)=_,_,%v, want _,_,nil", i, err)
		}
		if got, want := cmt.Size(), i+1; got != want {
			t.Fatalf("new tree size=%d, want %d", got, want)
		}
	}
	if a, b := imt.CurrentRoot().Hash(), mustGetRoot(t, cmt); !bytes.Equal(a, b) {
		t.Errorf("got in-memory root of %v, but compact tree has root %v", a, b)
	}
}

func TestRootHashForVariousTreeSizes(t *testing.T) {
	tests := []struct {
		size     int64
		wantRoot []byte
	}{
		{0, testonly.MustDecodeBase64("47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=")},
		{10, testonly.MustDecodeBase64("VjWMPSYNtCuCNlF/RLnQy6HcwSk6CIipfxm+hettA+4=")},
		{15, testonly.MustDecodeBase64("j4SulYmocFuxdeyp12xXCIgK6PekBcxzAIj4zbQzNEI=")},
		{16, testonly.MustDecodeBase64("c+4Uc6BCMOZf/v3NZK1kqTUJe+bBoFtOhP+P3SayKRE=")},
		{100, testonly.MustDecodeBase64("dUh9hYH88p0CMoHkdr1wC2szbhcLAXOejWpINIooKUY=")},
		{255, testonly.MustDecodeBase64("SmdsuKUqiod3RX2jyF2M6JnbdE4QuTwwipfAowI4/i0=")},
		{256, testonly.MustDecodeBase64("qFI0t/tZ1MdOYgyPpPzHFiZVw86koScXy9q3FU5casA=")},
		{1000, testonly.MustDecodeBase64("RXrgb8xHd55Y48FbfotJwCbV82Kx22LZfEbmBGAvwlQ=")},
		{4095, testonly.MustDecodeBase64("cWRFdQhPcjn9WyBXE/r1f04ejxIm5lvg40DEpRBVS0w=")},
		{4096, testonly.MustDecodeBase64("6uU/phfHg1n/GksYT6TO9aN8EauMCCJRl3dIK0HDs2M=")},
		{10000, testonly.MustDecodeBase64("VZcav65F9haHVRk3wre2axFoBXRNeUh/1d9d5FQfxIg=")},
		{65535, testonly.MustDecodeBase64("iPuVYJhP6SEE4gUFp8qbafd2rYv9YTCDYqAxCj8HdLM=")},
	}

	b64e := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

	for _, test := range tests {
		tree := NewTree(rfc6962.DefaultHasher)
		for i := int64(0); i < test.size; i++ {
			l := []byte{byte(i & 0xff), byte((i >> 8) & 0xff)}
			tree.AddLeaf(l, nil)
		}
		if gotRoot := mustGetRoot(t, tree); !bytes.Equal(gotRoot, test.wantRoot) {
			t.Errorf("Test (treesize=%v) got root %v, want %v", test.size, b64e(gotRoot), b64e(test.wantRoot))
		}
		t.Log(tree)
		if isPerfectTree(test.size) {
			// A perfect tree should have a single hash at the highest bit that is just
			// the root hash.
			hashes := tree.Hashes()
			for i, got := range hashes {
				var want []byte
				if i == (len(hashes) - 1) {
					want = mustGetRoot(t, tree)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("Test(treesize=%v).nodes[i]=%x, want %x", test.size, got, want)
				}
			}
		}
	}
}

func benchmarkAddLeafHash(b *testing.B, visit VisitFn) {
	const size = 1024
	for n := 0; n < b.N; n++ {
		tree := NewTree(rfc6962.DefaultHasher)
		for i := 0; i < size; i++ {
			l := []byte{byte(i & 0xff), byte((i >> 8) & 0xff)}
			if _, err := tree.AddLeaf(l, visit); err != nil {
				b.Fatalf("AddLeaf: %v", err)
			}
		}
	}
}

func BenchmarkAddLeafHash(b *testing.B) {
	benchmarkAddLeafHash(b, func(NodeID, []byte) {})
}

func BenchmarkAddLeafHashNoVisitor(b *testing.B) {
	benchmarkAddLeafHash(b, nil)
}

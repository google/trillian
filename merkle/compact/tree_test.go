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
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"testing"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
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
			tree := NewTree(rfc6962.NewSHA256())
			idx := 0
			for _, br := range tc.breaks {
				for ; idx < br; idx++ {
					if _, _, err := tree.AddLeaf(inputs[idx], func(int, int64, []byte) error {
						return nil
					}); err != nil {
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
					if got, want := tree.CurrentRoot(), roots[br-1]; !bytes.Equal(got, want) {
						t.Errorf("CurrentRoot()=%v, want %v", got, want)
					}
					if diff := pretty.Compare(tree.Hashes(), hashes[br-1]); diff != "" {
						t.Errorf("post-Hashes() diff:\n%v", diff)
					}
				} else {
					if got, want := tree.CurrentRoot(), testonly.EmptyMerkleTreeRootHash(); !bytes.Equal(got, want) {
						t.Errorf("CurrentRoot()=%x, want %x (empty)", got, want)
					}
				}
			}
		})
	}
}

func failingGetNodeFunc(_ int, _ int64) ([]byte, error) {
	return []byte{}, errors.New("bang")
}

// This returns something that won't result in a valid root hash match, doesn't really
// matter what it is but it must be correct length for an SHA256 hash as if it was real
func fixedHashGetNodeFunc(_ int, _ int64) ([]byte, error) {
	return []byte("12345678901234567890123456789012"), nil
}

func TestLoadingTreeFailsNodeFetch(t *testing.T) {
	_, err := NewTreeWithState(rfc6962.NewSHA256(), 237, failingGetNodeFunc, []byte("notimportant"))

	if err == nil || !strings.Contains(err.Error(), "bang") {
		t.Errorf("Did not return correctly on failed node fetch: %v", err)
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewTreeWithState(rfc6962.NewSHA256(), 237, fixedHashGetNodeFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
	_, ok := err.(RootHashMismatchError)

	if err == nil || !ok {
		t.Errorf("Did not return correct error type on root mismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("Error %q doesn't mention mismatch", err.Error())
	}
}

func nodeKey(d int, i int64) (string, error) {
	n, err := storage.NewNodeIDForTreeCoords(int64(d), i, 64)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

func TestCompactVsFullTree(t *testing.T) {
	imt := merkle.NewInMemoryMerkleTree(rfc6962.NewSHA256())
	nodes := make(map[string][]byte)

	for i := int64(0); i < 1024; i++ {
		cmt, err := NewTreeWithState(
			rfc6962.NewSHA256(),
			imt.LeafCount(),
			func(depth int, index int64) ([]byte, error) {
				k, err := nodeKey(depth, index)
				if err != nil {
					t.Errorf("failed to create nodeID: %v", err)
				}
				h := nodes[k]
				return h, nil
			}, imt.CurrentRoot().Hash())

		if err != nil {
			t.Errorf("interation %d: failed to create CMT with state: %v", i, err)
		}
		if a, b := imt.CurrentRoot().Hash(), cmt.CurrentRoot(); !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}

		newLeaf := []byte(fmt.Sprintf("Leaf %d", i))

		iSeq, iHash, err := imt.AddLeaf(newLeaf)
		if err != nil {
			t.Errorf("AddLeaf(): %v", err)
		}

		cSeq, cHash, err := cmt.AddLeaf(newLeaf,
			func(depth int, index int64, hash []byte) error {
				k, err := nodeKey(depth, index)
				if err != nil {
					return fmt.Errorf("failed to create nodeID: %v", err)
				}
				nodes[k] = hash
				return nil
			})
		if err != nil {
			t.Fatalf("mt update failed: %v", err)
		}

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
		if a, b := imt.CurrentRoot().Hash(), cmt.CurrentRoot(); !bytes.Equal(a, b) {
			t.Errorf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}
	}

	// Build another compact Merkle tree by incrementally adding the leaves to an empty tree.
	cmt := NewTree(rfc6962.NewSHA256())
	for i := int64(0); i < imt.LeafCount(); i++ {
		newLeaf := []byte(fmt.Sprintf("Leaf %d", i))
		seq, _, err := cmt.AddLeaf(newLeaf, func(depth int, index int64, hash []byte) error {
			return nil
		})
		if err != nil {
			t.Fatalf("AddLeaf(%d)=_,_,%v, want _,_,nil", i, err)
		}
		if seq != i {
			t.Fatalf("AddLeaf(%d)=%d, want %d", i, seq, i)
		}
	}
	if a, b := imt.CurrentRoot().Hash(), cmt.CurrentRoot(); !bytes.Equal(a, b) {
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
		tree := NewTree(rfc6962.NewSHA256())
		for i := int64(0); i < test.size; i++ {
			l := []byte{byte(i & 0xff), byte((i >> 8) & 0xff)}
			tree.AddLeaf(l, func(int, int64, []byte) error {
				return nil
			})
		}
		if gotRoot := tree.CurrentRoot(); !bytes.Equal(gotRoot, test.wantRoot) {
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
					want = tree.CurrentRoot()
				}
				if !bytes.Equal(got, want) {
					t.Errorf("Test(treesize=%v).nodes[i]=%x, want %x", test.size, got, want)
				}
			}
		}
	}
}

func BenchmarkAddLeaf(b *testing.B) {
	tree := NewTree(rfc6962.New(crypto.SHA256))
	for i := 0; i < b.N; i++ {
		l := []byte(fmt.Sprintf("This %x is leaf data that we made up %d", i, i))
		tree.AddLeaf(l, func(int, int64, []byte) error {
			return nil
		})
	}
}

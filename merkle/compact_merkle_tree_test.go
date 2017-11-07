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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/kylelemons/godebug/pretty"
)

func checkUnusedNodesInvariant(c *CompactMerkleTree) error {
	// The structure of this invariant check mirrors the structure in
	// NewCompactMerkleTreeWithState in which only the nodes which
	// should be present for a tree of given size are fetched from the
	// backing store via GetNodeFunc.
	size := c.size
	sizeBits := bitLen(size)
	if isPerfectTree(size) {
		for i, n := range c.nodes {
			expectNil := i != sizeBits-1
			if expectNil && n != nil {
				return fmt.Errorf("perfect Tree size %d has non-nil node at index %d, wanted nil", size, i)
			}
			if !expectNil && n == nil {
				return fmt.Errorf("perfect Tree size %d has nil node at index %d, wanted non-nil", size, i)
			}
		}
	} else {
		for depth := 0; depth < sizeBits; depth++ {
			if size&1 == 1 {
				if c.nodes[depth] == nil {
					return fmt.Errorf("imperfect Tree size %d has nil node at index %d, wanted non-nil", c.size, depth)
				}
			} else {
				if c.nodes[depth] != nil {
					return fmt.Errorf("imperfect Tree size %d has non-nil node at index %d, wanted nil", c.size, depth)
				}
			}
			size >>= 1
		}
	}
	return nil
}

func TestAddingLeaves(t *testing.T) {
	inputs := testonly.MerkleTreeLeafTestInputs()
	roots := testonly.MerkleTreeLeafTestRootHashes()
	hashes := testonly.CompactMerkleTreeLeafTestNodeHashes()

	// We test the "same" thing 3 different ways this is to ensure than any lazy
	// update strategy being employed by the implementation doesn't affect the
	// api-visible calculation of root & size.
	{
		// First tree, add nodes one-by-one
		tree := NewCompactMerkleTree(rfc6962.DefaultHasher)
		if got, want := tree.Size(), int64(0); got != want {
			t.Errorf("Size()=%d, want %d", got, want)
		}
		if got, want := tree.CurrentRoot(), testonly.EmptyMerkleTreeRootHash(); !bytes.Equal(got, want) {
			t.Errorf("CurrentRoot()=%x, want %x", got, want)
		}

		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, []byte) error {
				return nil
			})
			if err := checkUnusedNodesInvariant(tree); err != nil {
				t.Fatalf("UnusedNodesInvariant check failed: %v", err)
			}
			if got, want := tree.Size(), int64(i+1); got != want {
				t.Errorf("Size()=%d, want %d", got, want)
			}
			if got, want := tree.CurrentRoot(), roots[i]; !bytes.Equal(got, want) {
				t.Errorf("CurrentRoot()=%v, want %v", got, want)
			}
			if diff := pretty.Compare(tree.Hashes(), hashes[i]); diff != "" {
				t.Errorf("post-Hashes() diff:\n%v", diff)
			}
		}
	}

	{
		// Second tree, add nodes all at once
		tree := NewCompactMerkleTree(rfc6962.DefaultHasher)
		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, []byte) error {
				return nil
			})
			if err := checkUnusedNodesInvariant(tree); err != nil {
				t.Fatalf("UnusedNodesInvariant check failed: %v", err)
			}
		}
		if got, want := tree.Size(), int64(8); got != want {
			t.Errorf("Size()=%d, want %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[7]; !bytes.Equal(got, want) {
			t.Errorf("CurrentRoot()=%v, want %v", got, want)
		}
		if diff := pretty.Compare(tree.Hashes(), hashes[7]); diff != "" {
			t.Errorf("post-Hashes() diff:\n%v", diff)
		}
	}

	{
		// Third tree, add nodes in two chunks
		tree := NewCompactMerkleTree(rfc6962.DefaultHasher)
		for i := 0; i < 3; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, []byte) error {
				return nil
			})
			if err := checkUnusedNodesInvariant(tree); err != nil {
				t.Fatalf("UnusedNodesInvariant check failed: %v", err)
			}
		}
		if got, want := tree.Size(), int64(3); got != want {
			t.Errorf("Size()=%d, want %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[2]; !bytes.Equal(got, want) {
			t.Errorf("CurrentRoot()=%v, want %v", got, want)
		}
		if diff := pretty.Compare(tree.Hashes(), hashes[2]); diff != "" {
			t.Errorf("post-Hashes() diff:\n%v", diff)
		}

		for i := 3; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, []byte) error {
				return nil
			})
			if err := checkUnusedNodesInvariant(tree); err != nil {
				t.Fatalf("UnusedNodesInvariant check failed: %v", err)
			}
		}
		if got, want := tree.Size(), int64(8); got != want {
			t.Errorf("Size()=%d, want %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[7]; !bytes.Equal(got, want) {
			t.Errorf("CurrentRoot()=%v, want %v", got, want)
		}
		if diff := pretty.Compare(tree.Hashes(), hashes[7]); diff != "" {
			t.Errorf("post-Hashes() diff:\n%v", diff)
		}
	}
}

func failingGetNodeFunc(int, int64) ([]byte, error) {
	return []byte{}, errors.New("bang")
}

// This returns something that won't result in a valid root hash match, doesn't really
// matter what it is but it must be correct length for an SHA256 hash as if it was real
func fixedHashGetNodeFunc(int, int64) ([]byte, error) {
	return []byte("12345678901234567890123456789012"), nil
}

func TestLoadingTreeFailsNodeFetch(t *testing.T) {
	_, err := NewCompactMerkleTreeWithState(rfc6962.DefaultHasher, 237, failingGetNodeFunc, []byte("notimportant"))

	if err == nil || !strings.Contains(err.Error(), "bang") {
		t.Errorf("Did not return correctly on failed node fetch: %v", err)
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewCompactMerkleTreeWithState(rfc6962.DefaultHasher, 237, fixedHashGetNodeFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
	_, ok := err.(RootHashMismatchError)

	if err == nil || !ok {
		t.Errorf("Did not return correct error type on root mismatch: %v", err)
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
	imt := NewInMemoryMerkleTree(rfc6962.DefaultHasher)
	nodes := make(map[string][]byte)

	for i := int64(0); i < 1024; i++ {
		cmt, err := NewCompactMerkleTreeWithState(
			rfc6962.DefaultHasher,
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
}

func TestRootHashForVariousTreeSizes(t *testing.T) {
	tests := []struct {
		size     int64
		wantRoot []byte
	}{
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
		tree := NewCompactMerkleTree(rfc6962.DefaultHasher)
		for i := int64(0); i < test.size; i++ {
			l := []byte{byte(i & 0xff), byte((i >> 8) & 0xff)}
			tree.AddLeaf(l, func(int, int64, []byte) error {
				return nil
			})
		}
		if got, want := tree.CurrentRoot(), test.wantRoot; !bytes.Equal(got, want) {
			t.Errorf("Test (treesize=%v) got root %v, want %v", test.size, b64e(got), b64e(want))
		}
	}
}

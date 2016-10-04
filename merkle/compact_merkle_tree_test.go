package merkle

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
)

func getTree() *CompactMerkleTree {
	return NewCompactMerkleTree(NewRFC6962TreeHasher(trillian.NewSHA256()))
}

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
	hasheses := testonly.CompactMerkleTreeLeafTestNodeHashes()

	// We test the "same" thing 3 different ways this is to ensure than any lazy
	// update strategy being employed by the implementation doesn't affect the
	// api-visible calculation of root & size.
	{
		// First tree, add nodes one-by-one
		tree := getTree()
		if got, want := tree.Size(), int64(0); got != want {
			t.Errorf("Size()=%d, want %d", got, want)
		}
		if got, want := tree.CurrentRoot(), testonly.EmptyMerkleTreeRootHash(); !bytes.Equal(got, want) {
			t.Errorf("CurrentRoot()=%v, want %v", got, want)
		}

		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
			if err := checkUnusedNodesInvariant(tree); err != nil {
				t.Fatalf("UnusedNodesInvariant check failed: %v", err)
			}
			if got, want := tree.Size(), int64(i+1); got != want {
				t.Errorf("Size()=%d, want %d", got, want)
			}
			if got, want := tree.CurrentRoot(), roots[i]; !bytes.Equal(got, want) {
				t.Errorf("CurrentRoot()=%v, want %v", got, want)
			}
			if got, want := tree.Hashes(), hasheses[i]; !reflect.DeepEqual(got, want) {
				t.Errorf("Hashes()=%v, want %v", got, want)
			}
		}
	}

	{
		// Second tree, add nodes all at once
		tree := getTree()
		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
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
		if got, want := tree.Hashes(), hasheses[7]; !reflect.DeepEqual(got, want) {
			t.Errorf("Hashes()=%v, want %v", got, want)
		}
	}

	{
		// Third tree, add nodes in two chunks
		tree := getTree()
		for i := 0; i < 3; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
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
		if got, want := tree.Hashes(), hasheses[2]; !reflect.DeepEqual(got, want) {
			t.Errorf("Hashes()=%v, want %v", got, want)
		}

		for i := 3; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
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
		if got, want := tree.Hashes(), hasheses[7]; !reflect.DeepEqual(got, want) {
			t.Errorf("Hashes()=%v, want %v", got, want)
		}
	}
}

func failingGetNodeFunc(depth int, index int64) (trillian.Hash, error) {
	return trillian.Hash{}, errors.New("Bang!")
}

// This returns something that won't result in a valid root hash match, doesn't really
// matter what it is but it must be correct length for an SHA256 hash as if it was real
func fixedHashGetNodeFunc(depth int, index int64) (trillian.Hash, error) {
	return []byte("12345678901234567890123456789012"), nil
}

func TestLoadingTreeFailsNodeFetch(t *testing.T) {
	_, err := NewCompactMerkleTreeWithState(NewRFC6962TreeHasher(trillian.NewSHA256()), 237, failingGetNodeFunc, []byte("notimportant"))

	if err == nil || !strings.Contains(err.Error(), "Bang!") {
		t.Errorf("Did not return correctly on failed node fetch: %v", err)
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewCompactMerkleTreeWithState(NewRFC6962TreeHasher(trillian.NewSHA256()), 237, fixedHashGetNodeFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
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
	imt := NewInMemoryMerkleTree(NewRFC6962TreeHasher(trillian.NewSHA256()))
	nodes := make(map[string]trillian.Hash)

	for i := 0; i < 1024; i++ {
		cmt, err := NewCompactMerkleTreeWithState(
			NewRFC6962TreeHasher(trillian.NewSHA256()),
			int64(imt.LeafCount()),
			func(depth int, index int64) (trillian.Hash, error) {
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

		iSeq, iHash := imt.AddLeaf(newLeaf)

		cSeq, cHash := cmt.AddLeaf(newLeaf,
			func(depth int, index int64, hash trillian.Hash) {
				k, err := nodeKey(depth, index)
				if err != nil {
					t.Errorf("failed to create nodeID: %v", err)
				}
				nodes[k] = hash
			})

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
	tests := []struct{
		size int64
		wantRoot trillian.Hash
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
		tree := NewCompactMerkleTree(NewRFC6962TreeHasher(trillian.NewSHA256()))
		for i := int64(0); i < test.size; i++ {
			l := []byte{ byte(i & 0xff), byte((i >> 8) & 0xff) }
			tree.AddLeaf(l, func(int, int64, trillian.Hash) {})
		}
		if got, want := tree.CurrentRoot(), test.wantRoot; !bytes.Equal(got, want) {
			t.Errorf("Test (treesize=%v) got root %v, want %v", test.size, b64e(got), b64e(want))
		}
	}
}
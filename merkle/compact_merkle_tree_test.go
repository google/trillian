package merkle

import (
	"bytes"
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
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), testonly.EmptyMerkleTreeRootHash(); !bytes.Equal(got, want) {
			t.Fatalf("Got root of %v, expected %v", got, want)
		}

		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
			if got, want := tree.Size(), int64(i+1); got != want {
				t.Fatalf("Got size of %d, expected %d", got, want)
			}
			if got, want := tree.CurrentRoot(), roots[i]; !bytes.Equal(got, want) {
				t.Fatalf("Expected root of %v, got %v", got, want)
			}
			if got, want := tree.Hashes(), hasheses[i]; !reflect.DeepEqual(got, want) {
				t.Fatalf("Expected hashes %v, got %v", got, want)
			}
		}
	}

	{
		// Second tree, add nodes all at once
		tree := getTree()
		for i := 0; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
		}
		if got, want := tree.Size(), int64(8); got != want {
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[7]; !bytes.Equal(got, want) {
			t.Fatalf("Expected root of %v, got %v", got, want)
		}
		if got, want := tree.Hashes(), hasheses[7]; !reflect.DeepEqual(got, want) {
			t.Fatalf("Expected hashes %v, got %v", got, want)
		}
	}

	{
		// Third tree, add nodes in two chunks
		tree := getTree()
		for i := 0; i < 3; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
		}
		if got, want := tree.Size(), int64(3); got != want {
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[2]; !bytes.Equal(got, want) {
			t.Fatalf("Expected root of %v, got %v", got, want)
		}
		if got, want := tree.Hashes(), hasheses[2]; !reflect.DeepEqual(got, want) {
			t.Fatalf("Expected hashes %v, got %v", got, want)
		}

		for i := 3; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
		}
		if got, want := tree.Size(), int64(8); got != want {
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[7]; !bytes.Equal(got, want) {
			t.Fatalf("Expected root of %v, got %v", got, want)
		}
		if got, want := tree.Hashes(), hasheses[7]; !reflect.DeepEqual(got, want) {
			t.Fatalf("Expected hashes %v, got %v", got, want)
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
		t.Fatalf("Did not return correctly on failed node fetch: %v", err)
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data
	_, err := NewCompactMerkleTreeWithState(NewRFC6962TreeHasher(trillian.NewSHA256()), 237, fixedHashGetNodeFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
	_, ok := err.(RootHashMismatchError)

	if err == nil || !ok {
		t.Fatalf("Did not return correct error type on root mismatch: %v", err)
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
			t.Fatalf("interation %d: failed to create CMT with state: %v", i, err)
		}
		if a, b := imt.CurrentRoot().Hash(), cmt.CurrentRoot(); !bytes.Equal(a, b) {
			t.Fatalf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
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
			t.Fatalf("iteration %d: Got in-memory sequence number of %d, expected %d", i, got, want)
		}
		if int64(iSeq) != cSeq+1 {
			t.Fatalf("iteration %d: Got in-memory sequence number of %d but %d (zero based) from compact tree", i, iSeq, cSeq)
		}
		if a, b := iHash.Hash(), cHash; !bytes.Equal(a, b) {
			t.Fatalf("iteration %d: Got leaf hash %v from in-memory tree, but %v from compact tree", i, a, b)
		}
		if a, b := imt.CurrentRoot().Hash(), cmt.CurrentRoot(); !bytes.Equal(a, b) {
			t.Fatalf("iteration %d: Got in-memory root of %v, but compact tree has root %v", i, a, b)
		}

	}
}

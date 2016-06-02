package merkle

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/google/trillian"
)

// This data came from the C++ CT tests
// referenceMerkleInputs are the leaf data inputs to the tree for the first 7 leaves
var referenceMerkleInputs = [][]byte{{}, { 0x00 }, { 0x10} , { 0x20, 0x21 }, { 0x30, 0x31 }, { 0x40, 0x41, 0x42, 0x43 }, { 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57}}
// referenceRootHash7 is the expected root hash if the 7 elements above are added to the tree in order
var referenceRootHash7 = []byte{0xdd, 0xb8, 0x9b, 0xe4, 0x03, 0x80, 0x9e, 0x32, 0x57, 0x50, 0xd3, 0xd2, 0x63, 0xcd, 0x78, 0x92, 0x9c, 0x29, 0x42, 0xb7, 0x94, 0x2a, 0x34, 0xb7, 0x7e, 0x12, 0x2c, 0x95, 0x94, 0xa7, 0x4c, 0x8c}

func mustHexDecode(b string) trillian.Hash {
	r, err := hex.DecodeString(b)
	if err != nil {
		panic(err)
	}
	return r
}

func getInputs() []trillian.Hash {
	return []trillian.Hash{
		trillian.Hash(""), trillian.Hash("\x00"), trillian.Hash("\x10"), trillian.Hash("\x20\x21"), trillian.Hash("\x30\x31"),
		trillian.Hash("\x40\x41\x42\x43"), trillian.Hash("\x50\x51\x52\x53\x54\x55\x56\x57"),
		trillian.Hash("\x60\x61\x62\x63\x64\x65\x66\x67\x68\x69\x6a\x6b\x6c\x6d\x6e\x6f")}
}

func getTestRoots() []trillian.Hash {
	return []trillian.Hash{
		// constants from C++ test: https://github.com/google/certificate-transparency/blob/master/cpp/merkletree/merkle_tree_test.cc#L277
		mustHexDecode("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
		mustHexDecode("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"),
		mustHexDecode("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77"),
		mustHexDecode("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		mustHexDecode("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4"),
		mustHexDecode("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef"),
		mustHexDecode("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c"),
		mustHexDecode("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328")}
}

func emptyTreeHash() trillian.Hash {
	const sha256EmptyTreeHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	return mustHexDecode(sha256EmptyTreeHash)
}

func getTree() *CompactMerkleTree {
	return NewCompactMerkleTree(trillian.NewSHA256())
}

func TestAddingLeaves(t *testing.T) {
	inputs := getInputs()
	roots := getTestRoots()
	// We test the "same" thing 3 different ways this is to ensure than any lazy
	// update strategy being employed by the implementation doesn't affect the
	// api-visible calculation of root & size.
	{
		// First tree, add nodes one-by-one
		tree := getTree()
		if got, want := tree.Size(), int64(0); got != want {
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), emptyTreeHash(); !bytes.Equal(got, want) {
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

		for i := 3; i < 8; i++ {
			tree.AddLeaf(inputs[i], func(int, int64, trillian.Hash) {})
		}
		if got, want := tree.Size(), int64(8); got != want {
			t.Fatalf("Got size of %d, expected %d", got, want)
		}
		if got, want := tree.CurrentRoot(), roots[7]; !bytes.Equal(got, want) {
			t.Fatalf("Expected root of %v, got %v", got, want)
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

// This returns canned data for some nodes so we test that the right ones are accessed
// We expect to see nodes fetched at coords 0,6 1,2 and 2,0 for a 7 element tree
func cannedHashGetNodeFunc(depth int, index int64) (trillian.Hash, error) {
	fmt.Printf("%d %d\n", depth, index)
	hasher := NewTreeHasher(trillian.NewSHA256())

	if depth == 0 && index == 6 {
		// We want the last leaf hash
		return hasher.HashLeaf(referenceMerkleInputs[6]), nil
	}

	if depth == 1 && index == 2 {
		// We want leaves 4&5 hashed as children of that node
		return hasher.HashChildren(hasher.HashLeaf(referenceMerkleInputs[4]),
				hasher.HashLeaf(referenceMerkleInputs[5])), nil
	}

	if (depth == 2 && index == 0) {
		// We want the level two hash of the left side of the tree
		nodeHash1 := hasher.HashChildren(hasher.HashLeaf(referenceMerkleInputs[0]),
			hasher.HashLeaf(referenceMerkleInputs[1]))
		nodeHash2 := hasher.HashChildren(hasher.HashLeaf(referenceMerkleInputs[2]),
			hasher.HashLeaf(referenceMerkleInputs[3]))

		return hasher.HashChildren(nodeHash1, nodeHash2), nil
	}

	return nil, fmt.Errorf("Didn't expect to see a fetch for node %d,%d", depth, index)
}

func TestLoadingTreeFailsNodeFetch(t *testing.T) {
	_, err := NewCompactMerkleTreeWithState(trillian.NewSHA256(), 237, failingGetNodeFunc, []byte("notimportant"))

	if err == nil || !strings.Contains(err.Error(), "Bang!") {
		t.Fatalf("Did not return correctly on failed node fetch")
	}
}

func TestLoadingTreeFailsBadRootHash(t *testing.T) {
	// Supply a root hash that can't possibly match the result of the SHA 256 hashing on our dummy
	// data. Using data from C++ CT code for reference Merkle Tree
	_, err := NewCompactMerkleTreeWithState(trillian.NewSHA256(), 237, fixedHashGetNodeFunc, []byte("nomatch!nomatch!nomatch!nomatch!"))
	_, ok := err.(RootHashMismatchError)

	if err == nil || !ok {
		t.Fatalf("Did not return correct error type on root mismatch: %v", err)
	}
}

func TestLoadingTreeState(t *testing.T) {
	_, err := NewCompactMerkleTreeWithState(trillian.NewSHA256(), 7, cannedHashGetNodeFunc, referenceRootHash7)

	if err != nil {
		t.Fatalf("Failed to load tree state: %v", err)
	}
}
package merkle

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/mock"
)

func getSparseMerkleTreeReaderWithMockTX(rev int64) (*SparseMerkleTreeReader, *storage.MockMapTX) {
	tx := &storage.MockMapTX{}
	return NewSparseMerkleTreeReader(rev, NewMapHasher(NewRFC6962TreeHasher(trillian.NewSHA256())), tx), tx
}

func isRootNodeOnly(nodes []storage.NodeID) bool {
	return len(nodes) == 1 &&
		nodes[0].PrefixLenBits == 0
}

func getEmptyRootNode() storage.Node {
	return storage.Node{
		NodeID:       storage.NewEmptyNodeID(0),
		Hash:         mustDecode(sparseEmptyRootHashB64),
		NodeRevision: 0,
	}
}

func randomBytes(t *testing.T, n int) []byte {
	r := make([]byte, n)
	g, err := rand.Read(r)
	if g != n || err != nil {
		t.Fatalf("Failed to read %d bytes of entropy for path, read %d and got error: %v", n, g, err)
	}
	return r
}

func getRandomRootNode(t *testing.T, rev int64) storage.Node {
	return storage.Node{
		NodeID:       storage.NewEmptyNodeID(0),
		Hash:         randomBytes(t, 32),
		NodeRevision: rev,
	}
}

func getRandomNonRootNode(t *testing.T, rev int64) storage.Node {
	nodeID := storage.NewNodeIDFromHash(randomBytes(t, 32))
	// Make sure it's not a root node.
	nodeID.PrefixLenBits = int(1 + randomBytes(t, 1)[0]%254)
	return storage.Node{
		NodeID:       nodeID,
		Hash:         randomBytes(t, 32),
		NodeRevision: rev,
	}
}

func TestRootAtRevision(t *testing.T) {
	r, tx := getSparseMerkleTreeReaderWithMockTX(100)
	node := getRandomRootNode(t, 14)
	tx.On("GetMerkleNodes", int64(23), mock.MatchedBy(isRootNodeOnly)).Return([]storage.Node{node}, nil)
	root, err := r.RootAtRevision(23)
	if err != nil {
		t.Fatalf("Failed when calling RootAtRevision(23): %v", err)
	}
	if expected, got := root, node.Hash; !bytes.Equal(expected, got) {
		t.Fatalf("Expected root %v, got %v", expected, got)
	}
}

func TestRootAtUnknownRevision(t *testing.T) {
	r, tx := getSparseMerkleTreeReaderWithMockTX(100)
	tx.On("GetMerkleNodes", int64(23), mock.MatchedBy(isRootNodeOnly)).Return([]storage.Node{}, nil)
	_, err := r.RootAtRevision(23)
	if err != ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root an non-existent revision did not result in ErrNoSuchRevision: %v", err)
	}
}

func TestRootAtRevisionHasMultipleRoots(t *testing.T) {
	r, tx := getSparseMerkleTreeReaderWithMockTX(100)
	n1, n2 := getRandomRootNode(t, 14), getRandomRootNode(t, 15)
	tx.On("GetMerkleNodes", int64(23), mock.MatchedBy(isRootNodeOnly)).Return([]storage.Node{n1, n2}, nil)
	_, err := r.RootAtRevision(23)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root an non-existent revision did not result in error: %v", err)
	}
}

func TestRootAtRevisionCatchesFutureRevision(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	// Sanity checking in RootAtRevision should catch this node being incorrectly
	// returned by the storage layer.
	n1 := getRandomRootNode(t, rev+1)
	tx.On("GetMerkleNodes", int64(rev), mock.MatchedBy(isRootNodeOnly)).Return([]storage.Node{n1}, nil)
	_, err := r.RootAtRevision(rev)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root with corrupt node did not result in error: %v", err)
	}
}

func TestRootAtRevisionCatchesNonRootNode(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	// Sanity checking in RootAtRevision should catch this node being incorrectly
	// returned by the storage layer.
	n1 := getRandomNonRootNode(t, rev)
	tx.On("GetMerkleNodes", int64(rev), mock.MatchedBy(isRootNodeOnly)).Return([]storage.Node{n1}, nil)
	_, err := r.RootAtRevision(rev)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root with corrupt node did not result in error: %v", err)
	}
}

func TestInclusionProofForNullEntryInEmptyTree(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	tx.On("GetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.NodeID")).Return([]storage.Node{}, nil)
	const key = "SomeArbitraryKey"
	proof, err := r.InclusionProof(rev, []byte(key))
	if err != nil {
		t.Fatalf("Got error while retrieving inclusion proof: %v", err)
	}

	if expected, got := 256, len(proof); expected != got {
		t.Fatalf("Expected proof of len %d, but got len %d", expected, got)
	}

	treeHasher := NewRFC6962TreeHasher(trillian.NewSHA256())
	// Verify these are null hashes
	for i := len(proof) - 1; i > 0; i-- {
		expectedParent := treeHasher.HashChildren(proof[i], proof[i])
		if got := proof[i-1]; !bytes.Equal(expectedParent, got) {
			t.Fatalf("Expected proof[%d] to be %v, but got %v", i, expectedParent, got)
		}
	}
	// And hashing the zeroth proof element with itself should give us the empty root hash
	if expected, got := mustDecode(sparseEmptyRootHashB64), treeHasher.HashChildren(proof[0], proof[0]); !bytes.Equal(expected, got) {
		t.Fatalf("Expected to generate sparseEmptyRootHash using proof[0], but got %v", got)
	}
}

func TestInclusionProofGetsIncorrectNode(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	tx.On("GetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.NodeID")).Return([]storage.Node{getRandomNonRootNode(t, 34)}, nil)
	const key = "SomeArbitraryKey"
	_, err := r.InclusionProof(rev, []byte(key))
	if err == nil {
		t.Fatal("InclusionProof() should've returned an error due to incorrect node from storage layer")
	}
	if !strings.Contains(err.Error(), "expected node ID") {
		t.Fatalf("Saw unexpected error: %v", err)
	}
}

func TestInclusionProofPassesThroughStorageError(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	e := errors.New("Boo!")
	tx.On("GetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.NodeID")).Return([]storage.Node{}, e)
	_, err := r.InclusionProof(rev, []byte("Whatever"))
	if err != e {
		t.Fatal("InclusionProof() should've returned an error '%v', but got '%v'", e, err)
	}
}

func TestInclusionProofGetsTooManyNodes(t *testing.T) {
	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(rev)
	const key = "SomeArbitraryKey"
	keyHash := r.hasher.keyHasher([]byte(key))
	// going to return one too many nodes
	nodes := make([]storage.Node, 257, 257)
	// First build a plausible looking set of proof nodes.
	for i := 1; i < 256; i++ {
		nodes[255-i].NodeID = storage.NewNodeIDFromHash(keyHash)
		nodes[255-i].NodeID.PrefixLenBits = i + 1
		nodes[255-i].NodeID.SetBit(i, nodes[255-i].NodeID.Bit(i)^1)
	}
	// and then tack on some rubbish:
	nodes[256] = getRandomNonRootNode(t, 42)

	tx.On("GetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.NodeID")).Return(nodes, nil)
	_, err := r.InclusionProof(rev, []byte(key))
	if err == nil {
		t.Fatal("InclusionProof() should've returned an error due to extra unused node")
	}
	if !strings.Contains(err.Error(), "failed to consume") {
		t.Fatalf("Saw unexpected error: %v", err)
	}
}

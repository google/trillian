package merkle

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/mock"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write mem profile to file")
)

func maybeProfileCPU(t *testing.T) func() {
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			t.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		return pprof.StopCPUProfile
	}
	return func() {}
}

func maybeProfileMemory(t *testing.T) {
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			t.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}

// TODO(al): collect these test helpers together somewhere.
func mustDecodeB64(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

func getSparseMerkleTreeReaderWithMockTX(rev int64) (*SparseMerkleTreeReader, *storage.MockMapTX) {
	tx := &storage.MockMapTX{}
	return NewSparseMerkleTreeReader(rev, NewMapHasher(NewRFC6962TreeHasher(trillian.NewSHA256())), tx), tx
}

func getSparseMerkleTreeWriterWithMockTX(rev int64) (*SparseMerkleTreeWriter, *storage.MockMapTX) {
	tx := &storage.MockMapTX{}
	return NewSparseMerkleTreeWriter(rev, NewMapHasher(NewRFC6962TreeHasher(trillian.NewSHA256())), tx), tx
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

type sparseKeyValue struct {
	k, v string
}

type sparseTestVector struct {
	kv           []sparseKeyValue
	expectedRoot trillian.Hash
}

func testSparseTreeCalculatedRoot(t *testing.T, vec sparseTestVector) {
	const rev = 100
	w, tx := getSparseMerkleTreeWriterWithMockTX(rev)

	tx.On("SetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.Node")).Return(nil)

	leaves := make([]HashKeyValue, 0)
	for _, kv := range vec.kv {
		leaves = append(leaves, HashKeyValue{w.hasher.keyHasher([]byte(kv.k)), w.hasher.HashLeaf([]byte(kv.v))})
	}

	if err := w.SetLeaves(leaves); err != nil {
		t.Fatalf("Got error adding leaves: %v", err)
	}
	root, err := w.CalculateRoot()
	if err != nil {
		t.Fatalf("Failed to commit map changes: %v", err)
	}
	if expected, got := vec.expectedRoot, root; !bytes.Equal(expected, got) {
		t.Fatalf("Expected root:\n%s, but got root:\n%s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
}

func TestSparseMerkleTreeWriterEmptyTree(t *testing.T) {
	testSparseTreeCalculatedRoot(t, sparseTestVector{[]sparseKeyValue{}, mustDecodeB64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")})
}

func TestSparseMerkleTreeWriter(t *testing.T) {
	vec := sparseTestVector{
		[]sparseKeyValue{{"key1", "value1"}, {"key2", "value2"}, {"key3", "value3"}},
		mustDecodeB64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
	}
	testSparseTreeCalculatedRoot(t, vec)
}

func TestSparseMerkleTreeWriterBigBatch(t *testing.T) {
	defer maybeProfileCPU(t)()
	const rev = 100
	w, tx := getSparseMerkleTreeWriterWithMockTX(rev)

	tx.On("SetMerkleNodes", int64(rev), mock.AnythingOfType("[]storage.Node")).Return(nil)

	const batchSize = 1024
	const numBatches = 4
	for x := 0; x < numBatches; x++ {
		h := make([]HashKeyValue, batchSize)
		for y := 0; y < batchSize; y++ {
			h[y].HashedKey = w.hasher.keyHasher([]byte(fmt.Sprintf("key-%d-%d", x, y)))
			h[y].HashedValue = w.hasher.TreeHasher.HashLeaf([]byte(fmt.Sprintf("value-%d-%d", x, y)))
		}
		if err := w.SetLeaves(h); err != nil {
			t.Fatalf("Failed to batch %d: %v", x, err)
		}
	}
	root, err := w.CalculateRoot()
	if err != nil {
		t.Fatalf("Failed to calculate root hash: %v", err)
	}

	// calculated using python code.
	const expectedRootB64 = "Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA="
	if expected, got := mustDecodeB64(expectedRootB64), root; !bytes.Equal(expected, root) {
		// Error, not Fatal so that we get our benchmark results regardless of the
		// result - useful if you want to up the amount of data without having to
		// figure out the expected root!
		t.Errorf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
	maybeProfileMemory(t)
}

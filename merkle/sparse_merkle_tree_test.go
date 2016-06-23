package merkle

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/mock"
)

// This root was calculated with the C++/Python sparse merkle tree code in the
// github.com/google/certificate-transparency repo.
const sparseEmptyRootHashB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="

func mustDecode(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

type sparseRefValue struct {
	index *big.Int
	value []byte
}

func (s *sparseRefValue) String() string {
	return fmt.Sprintf("sparseRefValue{index: %b, value: %v", s.index, s.value)
}

type by func(a, b *sparseRefValue) bool

func (by by) Sort(values []sparseRefValue) {
	s := &valueSorter{
		values: values,
		by:     by,
	}
	sort.Sort(s)
}

type valueSorter struct {
	values []sparseRefValue
	by     func(a, b *sparseRefValue) bool
}

func (s *valueSorter) Len() int {
	return len(s.values)
}

func (s *valueSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
}

func (s *valueSorter) Less(i, j int) bool {
	return s.by(&s.values[i], &s.values[j])
}

type sparseReference struct {
	hasher          TreeHasher
	hStarEmptyCache []trillian.Hash
}

func newSparseReference() sparseReference {
	h := NewRFC6962TreeHasher(trillian.NewSHA256())
	return sparseReference{
		hasher:          h,
		hStarEmptyCache: []trillian.Hash{h.HashLeaf([]byte(""))},
	}
}

func indexLess(a, b *sparseRefValue) bool {
	return a.index.Cmp(b.index) < 0
}

func (s *sparseReference) HStar2(n int, values []sparseRefValue) (trillian.Hash, error) {
	by(indexLess).Sort(values)
	offset := big.NewInt(0)
	return s.HStar2b(n, values, 0, len(values), offset)
}

func (s *sparseReference) HStarEmpty(n int) (trillian.Hash, error) {
	if len(s.hStarEmptyCache) <= n {
		emptyRoot, err := s.HStarEmpty(n - 1)
		if err != nil {
			return nil, err
		}
		h := s.hasher.HashChildren(emptyRoot, emptyRoot)
		if len(s.hStarEmptyCache) != n {
			return nil, fmt.Errorf("cache wrong size - expected %d, but cache contains %d entries", n, len(s.hStarEmptyCache))
		}
		s.hStarEmptyCache = append(s.hStarEmptyCache, h)
	}
	if n >= len(s.hStarEmptyCache) {
		return nil, fmt.Errorf("cache wrong size - expected %d or more, but cache contains %d entries", n, len(s.hStarEmptyCache))
	}
	return s.hStarEmptyCache[n], nil
}

func (s *sparseReference) HStar2b(n int, values []sparseRefValue, lo, hi int, offset *big.Int) (trillian.Hash, error) {
	if n == 0 {
		if lo == hi {
			return s.hStarEmptyCache[0], nil
		}
		if hiLoDelta := hi - lo; hiLoDelta != 1 {
			return nil, fmt.Errorf("hi-lo is not 1, but %d", hiLoDelta)
		}
		return s.hasher.HashLeaf(values[lo].value), nil
	}
	if lo == hi {
		return s.HStarEmpty(n)
	}

	split := new(big.Int).Lsh(big.NewInt(1), uint(n-1))
	split.Add(split, offset)
	i := lo + sort.Search(hi-lo, func(i int) bool { return values[lo+i].index.Cmp(split) >= 0 })
	lhs, err := s.HStar2b(n-1, values, lo, i, offset)
	if err != nil {
		return nil, err
	}
	rhs, err := s.HStar2b(n-1, values, i, hi, split)
	if err != nil {
		return nil, err
	}
	return s.hasher.HashChildren(lhs, rhs), nil
}

// createSparseRefValues builds a list of sparseRefValue structs suitable for
// passing into a the HStar2 sparse reference implementation.
// The map keys will be SHA256 hashed before being added to the returned
// structs.
func createSparseRefValues(vs map[string]string) []sparseRefValue {
	r := []sparseRefValue{}
	for k := range vs {
		h := sha256.Sum256([]byte(k))
		r = append(r, sparseRefValue{
			index: new(big.Int).SetBytes(h[:]),
			value: []byte(vs[k]),
		})
	}
	return r
}

func TestReferenceEmptyRootKAT(t *testing.T) {
	s := newSparseReference()
	root, err := s.HStar2(s.hasher.Size()*8, []sparseRefValue{})
	if err != nil {
		t.Fatalf("Failed to calculate root: %v", err)
	}
	if expected, got := mustDecode(sparseEmptyRootHashB64), root; !bytes.Equal(expected, got) {
		t.Fatalf("Expected empty root:\n%v\nGot:\n%v", expected, got)
	}
}

func TestReferenceSimpleDataSetKAT(t *testing.T) {
	s := newSparseReference()
	vector := []struct{ k, v, rootB64 string }{
		{"a", "0", "nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0="},
		{"b", "1", "EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY="},
		{"a", "2", "2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o="},
	}

	m := make(map[string]string)
	for i, x := range vector {
		m[x.k] = x.v
		values := createSparseRefValues(m)
		root, err := s.HStar2(s.hasher.Size()*8, values)
		if err != nil {
			t.Fatalf("Failed to calculate root at iteration %d: %v", err, i)
		}
		if expected, got := mustDecode(x.rootB64), root; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root:\n%v\nGot:\n%v", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}
}

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

package merkle

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// For more information about how Sparse Merkle Trees work see the Revocation Transparency
// paper in the docs directory. Note that applications are not limited to X.509 certificates
// and this implementation handles arbitrary data.

// SparseMerkleTreeReader knows how to read data from a TreeStorage transaction
// to provide proofs etc.
type SparseMerkleTreeReader struct {
	tx           storage.ReadOnlyTreeTX
	hasher       MapHasher
	treeRevision int64
}

type newTXFunc func() (storage.TreeTX, error)

// SparseMerkleTreeWriter knows how to store/update a stored sparse merkle tree
// via a TreeStorage transaction.
type SparseMerkleTreeWriter struct {
	tx           storage.TreeTX
	hasher       MapHasher
	treeRevision int64
	tree         Subtree
}

type indexAndHash struct {
	index []byte
	hash  trillian.Hash
}

// rootHashOrError represents a (sub-)tree root hash, or an error which
// prevented the calculation from completing.
type rootHashOrError struct {
	hash trillian.Hash
	err  error
}

// Subtree is an interface which must be implemented by subtree workers.
// Currently there's only a locally sharded go-routine based implementation,
// the the idea is that an RPC based sharding implementation could be created
// and dropped in.
type Subtree interface {
	// SetLeaf sets a single leaf hash for integration into a sparse merkle tree.
	SetLeaf(index []byte, hash trillian.Hash) error

	// CalculateRoot instructs the subtree worker to start calculating the root
	// hash of its tree.  It is an error to call SetLeaf() after calling this
	// method.
	CalculateRoot()

	// RootHash returns the calculated root hash for this subtree, if the root
	// hash has not yet been calculated, this method will block until it is.
	RootHash() (trillian.Hash, error)
}

// getSubtreeFunc is essentially a factory method for getting child subtrees.
type getSubtreeFunc func(prefix []byte) (Subtree, error)

// subtreeWriter knows how to calculate and store nodes for a subtree.
type subtreeWriter struct {
	// prefix is the path to the root of this subtree in the full tree.
	// i.e. all paths/indices under this tree share the same prefix.
	prefix []byte

	// subtreeDepth is the number of levels this subtree contains.
	subtreeDepth int

	// leafQueue is the work-queue containing leaves to be integrated into the
	// subtree.
	leafQueue chan func() (*indexAndHash, error)

	// root is channel of size 1 from which the subtree root can be read once it
	// has been calculated.
	root chan rootHashOrError

	// childMutex protexts access to children.
	childMutex sync.RWMutex

	// children is a map of child-subtrees by stringified prefix.
	children map[string]Subtree

	tx           storage.TreeTX
	treeRevision int64

	treeHasher TreeHasher

	getSubtree getSubtreeFunc
}

// getOrCreateChildSubtree returns, or creates and returns, a subtree worker
// for the specified childPrefix.
func (s *subtreeWriter) getOrCreateChildSubtree(childPrefix []byte) (Subtree, error) {
	childPrefixStr := string(childPrefix)
	s.childMutex.Lock()
	defer s.childMutex.Unlock()

	subtree := s.children[childPrefixStr]
	var err error
	if subtree == nil {
		subtree, err = s.getSubtree(childPrefix)
		if err != nil {
			return nil, err
		}
		s.children[childPrefixStr] = subtree

		// Since a new subtree worker is being created we'll add a future to
		// to the leafQueue such that calculation of *this* subtree's root will
		// incorportate the newly calculated child subtree root.
		s.leafQueue <- func() (*indexAndHash, error) {
			// RootHash blocks until the root is available (or it's errored out)
			h, err := subtree.RootHash()
			if err != nil {
				return nil, err
			}
			return &indexAndHash{
				index: childPrefix,
				hash:  h,
			}, nil
		}
	}
	return subtree, nil
}

// SetLeaf sets a single leaf hash for incorporation into the sparse merkle
// tree.
func (s *subtreeWriter) SetLeaf(index []byte, hash trillian.Hash) error {
	indexLen := len(index) * 8

	switch {
	case indexLen < s.subtreeDepth:
		return fmt.Errorf("index length %d is < our depth %d", indexLen, s.subtreeDepth)

	case indexLen > s.subtreeDepth:
		childPrefix := index[:s.subtreeDepth/8]
		subtree, err := s.getOrCreateChildSubtree(childPrefix)
		if err != nil {
			return err
		}

		return subtree.SetLeaf(index[s.subtreeDepth/8:], hash)

	case indexLen == s.subtreeDepth:
		s.leafQueue <- func() (*indexAndHash, error) { return &indexAndHash{index: index, hash: hash}, nil }
		return nil
	}

	return fmt.Errorf("internal logic error in SetLeaf. index length: %d, subtreeDepth: %d", indexLen, s.subtreeDepth)
}

// CalculateRoot initiates the process of calculating the subtree root.
// The leafQueue is closed.
func (s *subtreeWriter) CalculateRoot() {
	close(s.leafQueue)

	for _, v := range s.children {
		v.CalculateRoot()
	}
}

// RootHash returns the calculated subtree root hash, blocking if necessary.
func (s *subtreeWriter) RootHash() (trillian.Hash, error) {
	r := <-s.root
	return r.hash, r.err
}

func nodeIDFromAddress(size int, prefix []byte, index *big.Int, depth int) storage.NodeID {
	ib := index.Bytes()
	t := make(trillian.Hash, size)
	copy(t, prefix)
	copy(t[len(prefix):], ib)
	n := storage.NewNodeIDFromHash(t)
	n.PrefixLenBits = len(prefix)*8 + depth
	return n
}

// buildSubtree is the worker function which calculates the root hash.
// The root chan will have had exactly one entry placed in it, and have been
// subsequently closed when this method exits.
func (s *subtreeWriter) buildSubtree() {
	defer close(s.root)

	leaves := make([]HStar2LeafHash, 0, len(s.leafQueue))

	for leaf := range s.leafQueue {
		ih, err := leaf()
		if err != nil {
			s.root <- rootHashOrError{hash: nil, err: err}
			return
		}
		leaves = append(leaves, HStar2LeafHash{Index: new(big.Int).SetBytes(ih.index), LeafHash: ih.hash})
	}

	// calculate new root, and intermediate nodes:
	hs2 := NewHStar2(s.treeHasher)
	nodesToStore := make([]storage.Node, 0)
	treeDepthOffset := (s.treeHasher.Size()-len(s.prefix))*8 - s.subtreeDepth
	root, err := hs2.HStar2Nodes(s.subtreeDepth, treeDepthOffset, leaves,
		func(depth int, index *big.Int) (trillian.Hash, error) {
			nodeID := nodeIDFromAddress(s.treeHasher.Size(), s.prefix, index, depth)
			nodes, err := s.tx.GetMerkleNodes(s.treeRevision, []storage.NodeID{nodeID})
			if err != nil {
				return nil, err
			}
			if len(nodes) == 0 {
				return nil, nil
			}
			if expected, got := nodeID, nodes[0].NodeID; !expected.Equivalent(got) {
				return nil, fmt.Errorf("expected node ID %s from storage, but got %s", expected, got)
			}
			if expected, got := s.treeRevision, nodes[0].NodeRevision; got > expected {
				return nil, fmt.Errorf("expected node revision <= %d, but got %d", expected, got)
			}
			return nodes[0].Hash, nil
		},
		func(depth int, index *big.Int, h trillian.Hash) error {
			nID := nodeIDFromAddress(s.treeHasher.Size(), s.prefix, index, depth)
			nodesToStore = append(nodesToStore,
				storage.Node{
					NodeID:       nID,
					Hash:         h,
					NodeRevision: s.treeRevision,
				})
			return nil
		})
	if err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}

	// write nodes back to storage
	if err := s.tx.SetMerkleNodes(s.treeRevision, nodesToStore); err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}
	if err := s.tx.Commit(); err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}

	// send calculated root hash
	s.root <- rootHashOrError{root, nil}
}

var (
	// ErrNoSuchRevision is returned when a request is made for information about
	// a tree revision which does not exist.
	ErrNoSuchRevision = errors.New("no such revision")
)

// NewSparseMerkleTreeReader returns a new SparseMerkleTreeReader, reading at
// the specified tree revision, using the passed in MapHasher for calculating
// and verifying tree hashes read via tx.
func NewSparseMerkleTreeReader(rev int64, h MapHasher, tx storage.ReadOnlyTreeTX) *SparseMerkleTreeReader {
	return &SparseMerkleTreeReader{
		tx:           tx,
		hasher:       h,
		treeRevision: rev,
	}
}

func leafQueueSize(depths []int) int {
	if len(depths) == 1 {
		return 1024
	}
	// for higher levels make sure we've got enough space if all leaves turn out
	// to be sub-tree futures...
	return 1 << uint(depths[0])
}

// newLocalSubtreeWriter creates a new local go-routing based subtree worker.
func newLocalSubtreeWriter(rev int64, prefix []byte, depths []int, newTX newTXFunc, h TreeHasher) (Subtree, error) {
	tx, err := newTX()
	if err != nil {
		return nil, err
	}
	tree := subtreeWriter{
		treeRevision: rev,
		prefix:       prefix,
		subtreeDepth: depths[0],
		leafQueue:    make(chan func() (*indexAndHash, error), leafQueueSize(depths)),
		root:         make(chan rootHashOrError, 1),
		children:     make(map[string]Subtree),
		tx:           tx,
		treeHasher:   h,
		getSubtree: func(p []byte) (Subtree, error) {
			myPrefix := append(prefix, p...)
			return newLocalSubtreeWriter(rev, myPrefix, depths[1:], newTX, h)
		},
	}
	// TODO(al): probably shouldn't be spawning go routines willy-nilly like
	// this, but it'll do for now.
	go tree.buildSubtree()
	return &tree, nil
}

// NewSparseMerkleTreeWriter returns a new SparseMerkleTreeWriter, which will
// write data back into the tree at the specified revision, using the passed
// in MapHasher to calulate/verify tree hashes, storing via tx.
func NewSparseMerkleTreeWriter(rev int64, h MapHasher, newTX newTXFunc) (*SparseMerkleTreeWriter, error) {
	tx, err := newTX()
	if err != nil {
		return nil, err
	}
	// TODO(al): allow the tree layering sizes to be customisable somehow.
	const topSubtreeSize = 8 // must be a multiple of 8 for now.
	tree, err := newLocalSubtreeWriter(rev, []byte{}, []int{topSubtreeSize, h.Size()*8 - topSubtreeSize}, newTX, h.TreeHasher)
	if err != nil {
		return nil, err
	}
	return &SparseMerkleTreeWriter{
		tx:           tx,
		hasher:       h,
		tree:         tree,
		treeRevision: rev,
	}, nil
}

// RootAtRevision returns the sparse merkle tree root hash at the specified
// revision, or ErrNoSuchRevision if the requested revision doesn't exist.
func (s SparseMerkleTreeReader) RootAtRevision(rev int64) (trillian.Hash, error) {
	rootNodeID := storage.NewEmptyNodeID(256)
	nodes, err := s.tx.GetMerkleNodes(rev, []storage.NodeID{rootNodeID})
	if err != nil {
		return nil, err
	}
	switch {
	case len(nodes) == 0:
		return nil, ErrNoSuchRevision
	case len(nodes) > 1:
		return nil, fmt.Errorf("expected 1 node, but got %d", len(nodes))
	}
	// Sanity check the nodeID
	if !nodes[0].NodeID.Equivalent(rootNodeID) {
		return nil, fmt.Errorf("unexpected node returned with ID: %v", nodes[0].NodeID)
	}
	// Sanity check the revision
	if nodes[0].NodeRevision > rev {
		return nil, fmt.Errorf("unexpected node revision returned: %d > %d", nodes[0].NodeRevision, rev)
	}
	return nodes[0].Hash, nil
}

// InclusionProof returns an inclusion (or non-inclusion) proof for the
// specified key at the specified revision.
// If the revision does not exist it will return ErrNoSuchRevision error.
func (s SparseMerkleTreeReader) InclusionProof(rev int64, key trillian.Key) ([]trillian.Hash, error) {
	kh := s.hasher.keyHasher(key)
	nid := storage.NewNodeIDFromHash(kh)
	sibs := nid.Siblings()
	nodes, err := s.tx.GetMerkleNodes(rev, sibs)
	if err != nil {
		return nil, err
	}
	// We're building a full proof from a combination of whichever nodes we got
	// back from the storage layer, and the set of "null" hashes.
	r := make([]trillian.Hash, len(sibs), len(sibs))
	ndx := 0
	// For each proof element:
	for i := 0; i < len(r); i++ {
		switch {
		case ndx >= len(nodes) || nodes[ndx].NodeID.PrefixLenBits > i:
			// we have no node for this level from storage, so use the null hash:
			r[i] = s.hasher.nullHashes[i]
		case nodes[ndx].NodeID.PrefixLenBits == i:
			// we've got a non-default node hash to use, but first just double-check it's the correct ID:
			if !sibs[i].Equivalent(nodes[ndx].NodeID) {
				return nil, fmt.Errorf("at level %d, expected node ID %v, but got %v", i, sibs[i].String(), nodes[ndx].NodeID.String())
			}
			// It is, copy it over
			r[i] = nodes[ndx].Hash
			// and move on to considering the next available returned node.
			ndx++
		default:
			// this should never happen under normal circumstances
			return nil, fmt.Errorf("internal error")
		}
	}
	// Make sure we used up all the returned nodes, otherwise something's gone wrong.
	if ndx != len(nodes) {
		return nil, fmt.Errorf("failed to consume all returned nodes; got %d nodes, but only used %d", len(nodes), ndx)
	}
	return r, nil
}

// SetLeaves adds a batch of leaves to the in-flight tree update.
func (s *SparseMerkleTreeWriter) SetLeaves(leaves []HashKeyValue) error {
	for _, l := range leaves {
		if err := s.tree.SetLeaf(l.HashedKey, l.HashedValue); err != nil {
			return err
		}
	}
	return nil
}

// CalculateRoot calculates the new root hash including the newly added leaves.
func (s *SparseMerkleTreeWriter) CalculateRoot() (trillian.Hash, error) {
	s.tree.CalculateRoot()
	return s.tree.RootHash()
}

// HashKeyValue represents a Hash(key)-Hash(value) pair.
type HashKeyValue struct {
	// HashedKey is the hash of the key data
	HashedKey trillian.Hash

	// HashedValue is the hash of the value data.
	HashedValue trillian.Hash
}

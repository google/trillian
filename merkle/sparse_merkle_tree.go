package merkle

import (
	"errors"
	"fmt"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// SparseMerkleTreeReader knows how to read data from a TreeStorage transaction
// to provide proofs etc.
type SparseMerkleTreeReader struct {
	tx           storage.ReadOnlyTreeTX
	hasher       MapHasher
	treeRevision int64
}

// SparseMerkleTreeWriter knows how to store/update a stored sparse merkle tree
// via a TreeStorage transaction.
type SparseMerkleTreeWriter struct {
	tx           storage.TreeTX
	hasher       MapHasher
	treeRevision uint64
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

// NewSparseMerkleTreeWriter returns a new SparseMerkleTreeWriter, which will
// write data back into the tree at the specified revision, using the passed
// in MapHasher to calulate/verify tree hashes, storing via tx.
func NewSparseMerkleTreeWriter(rev int64, h MapHasher, tx storage.TreeTX) *SparseMerkleTreeWriter {
	return &SparseMerkleTreeWriter{
		tx:     tx,
		hasher: h,
	}
}

// RootAtRevision returns the sparse merkle tree root hash at the specified
// revision, or ErrNoSuchRevision if the requested revision doesn't exist.
func (s SparseMerkleTreeReader) RootAtRevision(rev int64) (trillian.Hash, error) {
	nodes, err := s.tx.GetMerkleNodes(rev, []storage.NodeID{storage.NewEmptyNodeID(256)})
	if err != nil {
		return nil, err
	}
	switch {
	case len(nodes) == 0:
		return nil, ErrNoSuchRevision
	case len(nodes) > 1:
		return nil, fmt.Errorf("expected 1 node, but got %d", len(nodes))
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
	r := make([]trillian.Hash, len(sibs), len(sibs))
	ndx := 0
	for i := 0; i < len(r); i++ {
		switch {
		case ndx >= len(sibs) || sibs[ndx].PrefixLenBits > i:
			// we have no node for this level, so use the null hashes
			r[i] = s.hasher.nullHashes[i]
		case sibs[ndx].PrefixLenBits == i:
			// we've got a non-default node hash to use, but first just double-check it's the correct ID:
			if !sibs[ndx].Equivalent(nodes[i].NodeID) {
				return nil, fmt.Errorf("expected node ID %v, but got %v", sibs[ndx].String(), nodes[i].NodeID.String())
			}
			r[i] = nodes[ndx].Hash
			ndx++
		default:
			// this should never happen under normal circumstances
			return nil, fmt.Errorf("unexpected node; wanted level %d but got %v", i, sibs)
		}
		r[i] = nodes[i].Hash
	}
	if ndx+1 != len(sibs) {
		return nil, fmt.Errorf("failed to consume all returned nodes; got %d nodes, but only used %d", len(sibs), ndx+1)
	}
	return r, nil
}

// SetLeaves commits the tree to setting a batch leaves to the specified value.
// There may be a delay between the transaction this request is part of
// successfully commiting, and the updated values being reflected in the served tree.
func (s *SparseMerkleTreeWriter) SetLeaves(newRevision int64, leaves []HashKeyValue) (trillian.TreeRoot, error) {
	return trillian.TreeRoot{}, errors.New("unimplemented")
}

// HashKeyValue represents a Hash(key)-Hash(value) pair.
type HashKeyValue struct {
	// HashedKey is the hash of the key data
	HashedKey trillian.Hash

	// HashedValue is the hash of the value data.
	HashedValue trillian.Hash
}

package merkle

import (
	"errors"
	"fmt"

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

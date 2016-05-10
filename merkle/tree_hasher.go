package merkle

import (
	"github.com/google/trillian"
)

// Domain separation prefixes
const (
	LeafHashPrefix = 0
	NodeHashPrefix = 1
)

// TreeHasher is a set of domain separated hashers for creating merkle tree hashes.
type TreeHasher struct {
	trillian.Hasher
	leafHasher func([]byte) trillian.Hash
	nodeHasher func([]byte) trillian.Hash
}

// NewTreeHasher creates a new TreeHasher based on the passed in hash function.
func NewTreeHasher(hasher trillian.Hasher) TreeHasher {
	return TreeHasher{
		Hasher:     hasher,
		leafHasher: leafHasher(hasher),
		nodeHasher: nodeHasher(hasher),
	}
}

// HashLeaf returns the merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t TreeHasher) HashLeaf(leaf []byte) trillian.Hash {
	return t.leafHasher(leaf)
}

// HashChildren returns the inner merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t TreeHasher) HashChildren(l, r []byte) trillian.Hash {
	return t.nodeHasher(append(append([]byte{}, l...), r...))
}

type hashFunc func([]byte) trillian.Hash

// leafHasher builds a function to calculate leaf hashes based on the Hasher h.
func leafHasher(h trillian.Hasher) hashFunc {
	return func(b []byte) trillian.Hash {
		return h.Digest(append([]byte{LeafHashPrefix}, b...))
	}
}

// leafHasher builds a function to calculate internal node hashes based on the Hasher h.
func nodeHasher(h trillian.Hasher) hashFunc {
	return func(b []byte) trillian.Hash {
		return h.Digest(append([]byte{NodeHashPrefix}, b...))
	}
}

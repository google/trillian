package merkle

import (
	"github.com/google/trillian"
)

// MapHasher is a specialised TreeHasher which also knows about the set of
// "null" hashes for the unused sections of a SparseMerkleTree.
type MapHasher struct {
	TreeHasher
	keyHasher  keyHashFunc
	nullHashes []trillian.Hash
}

// NewMapHasher creates a new MapHasher based on the passed in hash function.
func NewMapHasher(hasher trillian.Hasher) MapHasher {
	th := NewTreeHasher(hasher)
	return MapHasher{
		TreeHasher: th,
		keyHasher:  keyHasher(hasher),
		nullHashes: createNullHashes(th),
	}
}

type keyHashFunc func([]byte) trillian.Hash

func keyHasher(h trillian.Hasher) keyHashFunc {
	return func(b []byte) trillian.Hash {
		return h.Digest(b)
	}
}

func createNullHashes(th TreeHasher) []trillian.Hash {
	numEntries := th.Size() * 8
	r := make([]trillian.Hash, numEntries, numEntries)
	r[numEntries-1] = th.HashLeaf([]byte{})
	for i := numEntries - 2; i >= 0; i-- {
		r[i] = th.HashChildren(r[i+1], r[i+1])
	}
	return r
}

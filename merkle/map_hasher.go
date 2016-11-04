package merkle

import (
	"github.com/google/trillian/crypto"
)

// MapHasher is a specialised TreeHasher which also knows about the set of
// "null" hashes for the unused sections of a SparseMerkleTree.
type MapHasher struct {
	TreeHasher
	HashKey    keyHashFunc
	nullHashes [][]byte
}

// NewMapHasher creates a new MapHasher based on the passed in hash function.
func NewMapHasher(th TreeHasher) MapHasher {
	return MapHasher{
		TreeHasher: th,
		HashKey:    keyHasher(th.Hasher),
		nullHashes: createNullHashes(th),
	}
}

type keyHashFunc func([]byte) []byte

func keyHasher(h crypto.Hasher) keyHashFunc {
	return func(b []byte) []byte {
		return h.Digest(b)
	}
}

func createNullHashes(th TreeHasher) [][]byte {
	numEntries := th.Size() * 8
	r := make([][]byte, numEntries, numEntries)
	r[numEntries-1] = th.HashLeaf([]byte{})
	for i := numEntries - 2; i >= 0; i-- {
		r[i] = th.HashChildren(r[i+1], r[i+1])
	}
	return r
}

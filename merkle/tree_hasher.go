package merkle

import (
	"github.com/google/trillian/crypto"
)

// TODO(al): investigate whether we need configurable TreeHashers for
// different users. Apparently E2E hashes in tree-level to the internal nodes
// for example, and some users may want different domain separation prefixes
// etc.
//
// BIG SCARY COMMENT:
//
// We don't want this code to have to depend on or constrain implementations for
// specific applications but we haven't decided how we're going to split domain
// specific stuff from the generic yet and we don't want to lose track of the fact
// that this hashing needs to be domain aware to some extent.
//
// END OF BIG SCARY COMMENT

// Domain separation prefixes
// TODO(Martin2112): Move anything CT specific out of here to <handwave> look over there
const (
	RFC6962LeafHashPrefix = 0
	RFC6962NodeHashPrefix = 1
)

// TreeHasher is a set of domain separated hashers for creating Merkle tree hashes.
type TreeHasher struct {
	crypto.Hasher
	leafHasher  func([]byte) []byte
	nodeHasher  func([]byte) []byte
	emptyHasher func() []byte
}

// NewRFC6962TreeHasher creates a new TreeHasher based on the passed in hash function.
// TODO(Martin2112): Move anything CT specific out of here to <handwave> look over there
func NewRFC6962TreeHasher(hasher crypto.Hasher) TreeHasher {
	return TreeHasher{
		Hasher:      hasher,
		leafHasher:  rfc6962LeafHasher(hasher),
		nodeHasher:  rfc6962NodeHasher(hasher),
		emptyHasher: rfc6962EmptyHasher(hasher),
	}
}

// HashEmpty returns the hash of an empty element for the tree
func (t TreeHasher) HashEmpty() []byte {
	return t.emptyHasher()
}

// HashLeaf returns the Merkle tree leaf hash of the data passed in through leaf.
// The data in leaf is prefixed by the LeafHashPrefix.
func (t TreeHasher) HashLeaf(leaf []byte) []byte {
	return t.leafHasher(leaf)
}

// HashChildren returns the inner Merkle tree node hash of the the two child nodes l and r.
// The hashed structure is NodeHashPrefix||l||r.
func (t TreeHasher) HashChildren(l, r []byte) []byte {
	return t.nodeHasher(append(append([]byte{}, l...), r...))
}

type emptyHashFunc func() []byte
type hashFunc func([]byte) []byte

// rfc6962EmptyHasher builds a function to calculate the hash of an empty element for CT
func rfc6962EmptyHasher(h crypto.Hasher) emptyHashFunc {
	return func() []byte {
		return h.Digest([]byte{})
	}
}

// rfc6962LeafHasher builds a function to calculate leaf hashes based on the Hasher h for CT.
func rfc6962LeafHasher(h crypto.Hasher) hashFunc {
	return func(b []byte) []byte {
		return h.Digest(append([]byte{RFC6962LeafHashPrefix}, b...))
	}
}

// rfc6962NodeHasher builds a function to calculate internal node hashes based on the Hasher h for CT.
func rfc6962NodeHasher(h crypto.Hasher) hashFunc {
	return func(b []byte) []byte {
		return h.Digest(append([]byte{RFC6962NodeHashPrefix}, b...))
	}
}

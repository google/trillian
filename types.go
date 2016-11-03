// Package trillian provides common data structures and interfaces used throughout Trillian.
package trillian

import (
	"encoding/base64"
)

// Hash repesents the cryptographic hash value of some data
type Hash []byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h)
}

// Leaf represents the data behind Merkle leaves.
type Leaf struct {
	// MerkleLeafHash is the tree hash of LeafValue
	MerkleLeafHash Hash
	// LeafValue is the data the tree commits to.
	LeafValue      []byte
	// ExtraData holds related contextual data, but this data is not included in any hash.
	ExtraData      []byte
	// TODO(Martin2112): Add a separate field for LeafValueHash and wire it up to API
}

// LogLeaf represents data behind Log leaves.
type LogLeaf struct {
	// Leaf holds the the leaf data itself.
	Leaf
	// SequenceNumber holds the position in the log this leaf has been assigned to.
	SequenceNumber int64
}

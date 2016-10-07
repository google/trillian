// Package trillian provides common data structures and interfaces used throughout Trillian.
package trillian

import (
	"encoding/base64"
	"time"
)

// Hash repesents the cryptographic hash value of some data
type Hash []byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h)
}

// MapID represents a single Map instance, and ties it to a particular stored tree instance.
type MapID struct {
	// MapID is the unique (public) ID of the Map.
	MapID []byte
	// TreeID is the internal ID of the stored tree data.
	TreeID int64
}

// LogID represents a single Log instance, and ties it to a particular stored tree instance.
type LogID struct {
	// LogID is the unique (public) ID of the Log.
	LogID []byte
	// TreeID is the internal ID of the stored tree data.
	TreeID int64
}

// TreeRoot represents the root of a Merkle tree.
type TreeRoot struct {
	// RootHash is the Merkle tree root hash.
	RootHash Hash
	// Timestamp is the instant at which the root was calculated.
	Timestamp time.Time
	// TreeID identifies the particular tree this root pertains to.
	TreeID int64
	// TreeRevision is effectively a "sequence" number for TreeRoots.
	TreeRevision int64
}

// Leaf represents the data behind merkle leaves.
type Leaf struct {
	// LeafHash is the tree hash of LeafValue
	LeafHash Hash
	// LeafValue is the data the tree commits to.
	LeafValue []byte
	// ExtraData holds related contextual data, but this data is not included in any hash.
	ExtraData []byte
}

// LogLeaf represents data behind Log leaves.
type LogLeaf struct {
	// Leaf holds the the leaf data itself.
	Leaf
	// SequenceNumber holds the position in the log this leaf has been assigned to.
	SequenceNumber int64
}

// Key is a map key.
type Key []byte

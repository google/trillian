package storage

import "github.com/google/trillian/merkle/compact"

// Node represents a Merkle tree node.
type Node struct {
	ID   compact.NodeID
	Hash []byte
}

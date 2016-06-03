package storage

import (
	"bytes"
	"errors"

	"github.com/google/trillian"
)

// ErrReadOnly is returned when operations are not allowed because a resource is read only
var ErrReadOnly = errors.New("storage: Operation not allowed because resource is read only")

// Node represents a single node in a Merkle tree.
type Node struct {
	NodeID       NodeID
	Hash         trillian.Hash
	NodeRevision int64
}

// NodeID uniquely identifies a Node within a versioned MerkleTree.
type NodeID struct {
	// path is effectively a bit set, with path[0] being the LSB (identifying the
	// leaf child), and successive bits identifying the higher level children up
	// to the root.
	Path []byte
	// PrefixLenBits is the number of MSB in Path which are considered part of
	// this NodeID.
	//
	// e.g. if Path contains two bytes, and PrefixLenBits is 9, then the 8 bits
	// in Path[1] are included, along with the highest bit of Path[0]
	PrefixLenBits int
}

// bytesForBits returns the number of bytes required to store numBits bits.
func bytesForBits(numBits int) int {
	numBytes := numBits / 8
	if numBits%8 != 0 {
		numBytes++
	}
	return numBytes
}

// NewEmptyNodeID creates a new zero-length NodeID with sufficient underlying
// capacity to store a maximum of maxLenBits.
func NewEmptyNodeID(maxLenBits int) NodeID {
	return NodeID{
		Path:          make([]byte, bytesForBits(maxLenBits)),
		PrefixLenBits: 0,
	}
}

// NewNodeIDWithPrefix creates a new NodeID of nodeIDLen bits with the prefixLen MSBs set to prefix.
func NewNodeIDWithPrefix(prefix uint64, prefixLenBits, nodeIDLenBits, maxLenBits int) NodeID {
	maxLenBytes := bytesForBits(maxLenBits)
	p := NodeID{
		Path:          make([]byte, maxLenBytes),
		PrefixLenBits: nodeIDLenBits,
	}

	bit := maxLenBits - prefixLenBits
	for i := 0; i < prefixLenBits; i++ {
		if prefix&1 != 0 {
			p.SetBit(bit, 1)
		}
		bit++
		prefix >>= 1
	}
	return p
}

// NewNodeIDForTreeCoords creates a new NodeID for a Tree node with a specified depth and
// index
func NewNodeIDForTreeCoords(depth int64, index int64, maxLenBits int) NodeID {
	r := NewEmptyNodeID(maxLenBits)
	for i := 0; index > 0; i++ {
		r.Path[i] = byte(index & 0xff)
		index >>= 8
	}
	r.PrefixLenBits = int(depth)
	return r
}

// SetBit sets the ith bit to true if b is non-zero, and false otherwise.
func (n *NodeID) SetBit(i int, b uint) {
	if b == 0 {
		n.Path[i/8] &= ^(1 << uint(i%8))
	} else {
		n.Path[i/8] |= (1 << uint(i%8))
	}
}

// Bit returns 1 if the ith bit is true, and false otherwise.
func (n *NodeID) Bit(i int) uint {
	return uint((n.Path[i/8] >> uint(i%8)) & 0x01)
}

// String returns a string representation of the binary value of the NodeID.
// The left-most bit is the MSB (i.e. nearer the root of the tree).
func (n *NodeID) String() string {
	var r bytes.Buffer
	for i := n.PrefixLenBits - 1; i >= 0; i-- {
		r.WriteRune(rune('0' + n.Bit(i)))
	}
	return r.String()
}

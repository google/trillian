package storage

import (
	"bytes"
	"errors"

	"github.com/google/trillian"
)

// ErrReadOnly is returned when storage operations are not allowed because a resource is read only
var ErrReadOnly = errors.New("storage: Operation not allowed because resource is read only")

// Node represents a single node in a Merkle tree.
type Node struct {
	NodeID       NodeID
	Hash         trillian.Hash
	NodeRevision int64
}

// NodeID uniquely identifies a Node within a versioned MerkleTree.
type NodeID struct {
	// path is effectively a BigEndian bit set, with path[0] being the MSB
	// (identifying the root child), and successive bits identifying the lower
	// level children down to the leaf.
	Path []byte
	// PrefixLenBits is the number of MSB in Path which are considered part of
	// this NodeID.
	//
	// e.g. if Path contains two bytes, and PrefixLenBits is 9, then the 8 bits
	// in Path[0] are included, along with the lowest bit of Path[1]
	PrefixLenBits int
	PathLenBits   int
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
func NewNodeIDFromHash(h trillian.Hash) NodeID {
	return NodeID{
		Path:          h,
		PathLenBits:   len(h) * 8,
		PrefixLenBits: len(h) * 8,
	}
}

func NewEmptyNodeID(maxLenBits int) NodeID {
	return NodeID{
		Path:          make([]byte, bytesForBits(maxLenBits)),
		PrefixLenBits: 0,
		PathLenBits:   maxLenBits,
	}
}

// NewNodeIDWithPrefix creates a new NodeID of nodeIDLen bits with the prefixLen MSBs set to prefix.
func NewNodeIDWithPrefix(prefix uint64, prefixLenBits, nodeIDLenBits, maxLenBits int) NodeID {
	maxLenBytes := bytesForBits(maxLenBits)
	p := NodeID{
		Path:          make([]byte, maxLenBytes),
		PrefixLenBits: nodeIDLenBits,
		PathLenBits:   maxLenBits,
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
// index.
// This method is used exclusively by the Log, and since the Log model grows upwards from the
// leaves we modify the provided coords accordingly.
func NewNodeIDForTreeCoords(depth int64, index int64, maxLenBits int) NodeID {
	r := NewEmptyNodeID(maxLenBits)
	for i := len(r.Path) - 1; index > 0; i-- {
		r.Path[i] = byte(index & 0xff)
		index >>= 8
	}
	r.PrefixLenBits = int(int64(maxLenBits) - depth)
	return r
}

// SetBit sets the ith bit to true if b is non-zero, and false otherwise.
func (n *NodeID) SetBit(i int, b uint) {
	// TODO(al): investigate whether having lookup tables for these might be
	// faster.
	bIndex := (n.PathLenBits - i - 1) / 8
	if b == 0 {
		n.Path[bIndex] &= ^(1 << uint(i%8))
	} else {
		n.Path[bIndex] |= (1 << uint(i%8))
	}
}

// Bit returns 1 if the ith bit is true, and false otherwise.
func (n *NodeID) Bit(i int) uint {
	bIndex := (n.PathLenBits - i - 1) / 8
	return uint((n.Path[bIndex] >> uint(i%8)) & 0x01)
}

// String returns a string representation of the binary value of the NodeID.
// The left-most bit is the MSB (i.e. nearer the root of the tree).
func (n *NodeID) String() string {
	var r bytes.Buffer
	limit := n.PathLenBits - n.PrefixLenBits
	for i := n.PathLenBits - 1; i >= limit; i-- {
		r.WriteRune(rune('0' + n.Bit(i)))
	}
	return r.String()
}

func (n *NodeID) Siblings() []NodeID {
	r := make([]NodeID, n.PrefixLenBits, n.PrefixLenBits)
	l := n.PrefixLenBits
	// Index of the bit to twiddle:
	bi := n.PathLenBits - n.PrefixLenBits
	for i := 0; i < len(r); i++ {
		r[i].PrefixLenBits = l - i
		r[i].Path = make([]byte, len(n.Path))
		r[i].PathLenBits = n.PathLenBits
		copy(r[i].Path, n.Path)
		r[i].SetBit(bi, n.Bit(bi)^1)
		bi++
	}
	return r
}

func (n *NodeID) AsProto() *NodeIDProto {
	return &NodeIDProto{Path: n.Path, PrefixLenBits: int32(n.PrefixLenBits)}
}

func NewNodeIDFromProto(p NodeIDProto) *NodeID {
	return &NodeID{
		Path:          p.Path,
		PrefixLenBits: int(p.PrefixLenBits),
		PathLenBits:   len(p.Path) * 8,
	}
}

// Equivalent return true iff the other represents the same path prefix as this NodeID.
func (n *NodeID) Equivalent(other NodeID) bool {
	return n.String() == other.String()
}

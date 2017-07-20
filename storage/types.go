// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"bytes"
	"fmt"

	"github.com/google/trillian/storage/storagepb"
)

// Integer types to distinguish storage errors that might need to be mapped at a higher level.
const (
	DuplicateLeaf = iota
)

// Error is a typed error that the storage layer can return to give callers information
// about the error to decide how to handle it.
type Error struct {
	ErrType int
	Detail  string
	Cause   error
}

// Error formats the internal details of an Error including the original cause.
func (s Error) Error() string {
	return fmt.Sprintf("Storage: %d: %s: %v", s.ErrType, s.Detail, s.Cause)
}

// Node represents a single node in a Merkle tree.
type Node struct {
	NodeID       NodeID
	Hash         []byte
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
	return (numBits + 7) >> 3
}

// NewNodeIDFromHash creates a new NodeID for the given Hash.
func NewNodeIDFromHash(h []byte) NodeID {
	return NodeID{
		Path:          h,
		PathLenBits:   len(h) * 8,
		PrefixLenBits: len(h) * 8,
	}
}

// NewEmptyNodeID creates a new zero-length NodeID with sufficient underlying
// capacity to store a maximum of maxLenBits.
func NewEmptyNodeID(maxLenBits int) NodeID {
	return NodeID{
		Path:          make([]byte, bytesForBits(maxLenBits)),
		PrefixLenBits: 0,
		PathLenBits:   maxLenBits,
	}
}

// NewNodeIDWithPrefix creates a new NodeID of nodeIDLen bits with the prefixLen MSBs set to prefix.
// NewNodeIDWithPrefix places the lower prefixLenBits of prefix in the most significant bits of path.
// Path will have enough bytes to hold maxLenBits
//
func NewNodeIDWithPrefix(prefix uint64, prefixLenBits, nodeIDLenBits, maxLenBits int) NodeID {
	if got, want := nodeIDLenBits%8, 0; got != want {
		panic(fmt.Sprintf("nodeIDLenBits mod 8: %v, want %v", got, want))
	}
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

func bitLen(x int64) int {
	r := 0
	for x > 0 {
		r++
		x >>= 1
	}
	return r
}

// NewNodeIDForTreeCoords creates a new NodeID for a Tree node with a specified depth and
// index.
// This method is used exclusively by the Log, and, since the Log model grows upwards from the
// leaves, we modify the provided coords accordingly.
//
// depth is the Merkle tree level: 0 = leaves, and increases upwards towards the root.
//
// index is the horizontal index into the tree at level depth, so the returned
// NodeID will be zero padded on the right by depth places.
func NewNodeIDForTreeCoords(depth int64, index int64, maxPathBits int) (NodeID, error) {
	bl := bitLen(index)
	if index < 0 || depth < 0 || bl > int(maxPathBits-int(depth)) {
		return NodeID{}, fmt.Errorf("depth/index combination out of range: depth=%d index=%d", depth, index)
	}
	// This node is effectively a prefix of the subtree underneath (for non-leaf
	// depths), so we shift the index accordingly.
	uidx := uint64(index) << uint(depth)
	r := NewEmptyNodeID(maxPathBits)
	for i := len(r.Path) - 1; uidx > 0 && i >= 0; i-- {
		r.Path[i] = byte(uidx & 0xff)
		uidx >>= 8
	}
	// In the storage model nodes closer to the leaves have longer nodeIDs, so
	// we "reverse" depth here:
	r.PrefixLenBits = int(maxPathBits - int(depth))
	return r, nil
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
	if got, want := i, n.PathLenBits-1; got > want {
		panic(fmt.Sprintf("storage: Bit(%v) > (PathLenBits -1): %v", got, want))
	}
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

// CoordString returns a string representation assuming that the NodeID represents a
// tree coordinate. Using this on a NodeID for a sparse Merkle tree will give incorrect
// results. Intended for debugging purposes, the format could change.
func (n *NodeID) CoordString() string {
	d := uint64(n.PathLenBits - n.PrefixLenBits)
	i := uint64(0)
	for _, p := range n.Path {
		i = (i << uint64(8)) + uint64(p)
	}

	return fmt.Sprintf("[d:%d, i:%d]", d, i>>d)
}

// Siblings returns the siblings of the given node.
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

// Equivalent return true iff the other represents the same path prefix as this NodeID.
func (n *NodeID) Equivalent(other NodeID) bool {
	return n.String() == other.String()
}

// PopulateSubtreeFunc is a function which knows how to re-populate a subtree
// from just its leaf nodes.
type PopulateSubtreeFunc func(*storagepb.SubtreeProto) error

// PrepareSubtreeWriteFunc is a function that carries out any required tree type specific
// manipulation of a subtree before it's written to storage
type PrepareSubtreeWriteFunc func(*storagepb.SubtreeProto) error

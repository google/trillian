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
	"math/big"

	"github.com/google/trillian/node"
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
	NodeID       node.Node
	Hash         []byte
	NodeRevision int64
}

// bytesForBits returns the number of bytes required to store numBits bits.
func bytesForBits(numBits int) int {
	return (numBits + 7) >> 3
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

// PopulateSubtreeFunc is a function which knows how to re-populate a subtree
// from just its leaf nodes.
type PopulateSubtreeFunc func(*storagepb.SubtreeProto) error

// PrepareSubtreeWriteFunc is a function that carries out any required tree type specific
// manipulation of a subtree before it's written to storage
type PrepareSubtreeWriteFunc func(*storagepb.SubtreeProto) error

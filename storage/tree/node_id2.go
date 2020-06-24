// Copyright 2019 Google LLC. All Rights Reserved.
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

package tree

import "fmt"

// NodeID2 identifies a node of a Merkle tree. It is a bit string that counts
// the node down from the tree root, i.e. 0 and 1 bits represent going to the
// left or right child correspondingly.
//
// NodeID2 is immutable, comparable, and can be used as a Golang map key. It
// also incurs zero memory allocations in transforming methods like Prefix and
// Sibling.
//
// The internal structure of NodeID2 is driven by its use-cases:
// - To make NodeID2 objects immutable and comparable, the Golang string type
//   is used for storing the bit string bytes.
// - To make Sibling and Prefix operations fast, the last byte is stored
//   separately from the rest of the bytes, so that it can be "amended".
// - To make NodeID2 objects comparable, there is only one (canonical) way to
//   encode an ID. For example, if the last byte is used partially, its unused
//   bits are always unset. See invariants next to field definitions below.
//
// Constructors and methods of NodeID2 make sure its invariants are always met.
//
// For example, an 11-bit node ID [1010,1111,001] is structured as follows:
// - path string contains 1 byte, which is [1010,1111].
// - last byte is [0010,0000]. Note the unset lower 5 bits.
// - bits is 3, so effectively only the upper 3 bits [001] of last are used.
//
// TODO(pavelkalinnikov, v2): Replace NodeID with this type.
type NodeID2 struct {
	path string
	last byte  // Invariant: Lowest (8-bits) bits of the last byte are unset.
	bits uint8 // Invariant: 1 <= bits <= 8, or bits == 0 for the empty ID.
}

// NewNodeID2 creates a NodeID2 from the given path bytes truncated to the
// specified number of bits if necessary. Panics if the number of bits is more
// than the byte string contains.
func NewNodeID2(path string, bits uint) NodeID2 {
	if bits == 0 {
		return NodeID2{}
	} else if mx := uint(len(path)) * 8; bits > mx {
		panic(fmt.Sprintf("NewNodeID2: bits %d > %d", bits, mx))
	}
	bytes, tailBits := split(bits)
	// Note: Getting the substring is cheap because strings are immutable in Go.
	return newMaskedNodeID2(path[:bytes], path[bytes], tailBits)
}

// NewNodeID2WithLast creates a NodeID2 from the given path bytes and the
// additional last byte, of which only the specified number of most significant
// bits is used. The number of bits must be between 1 and 8, and can be 0 only
// if the path bytes string is empty; otherwise the function panics.
func NewNodeID2WithLast(path string, last byte, bits uint8) NodeID2 {
	if bits > 8 {
		panic(fmt.Sprintf("NewNodeID2WithLast: bits %d > 8", bits))
	} else if bits == 0 && len(path) != 0 {
		panic("NewNodeID2WithLast: bits=0, but path is not empty")
	}
	return newMaskedNodeID2(path, last, bits)
}

// newMaskedNodeID2 constructs a NodeID ensuring its invariants are met. The
// last byte is masked so that the given number of upper bits are in use, and
// the others are unset.
func newMaskedNodeID2(path string, last byte, bits uint8) NodeID2 {
	last &= ^byte(1<<(8-bits) - 1) // Unset the unused bits.
	return NodeID2{path: path, last: last, bits: bits}
}

// BitLen returns the length of the NodeID2 in bits.
func (n NodeID2) BitLen() uint {
	return uint(len(n.path))*8 + uint(n.bits)
}

// FullBytes returns the ID bytes that are complete. Note that there might
// still be up to 8 extra bits, which can be obtained with the LastByte method.
func (n NodeID2) FullBytes() string {
	return n.path
}

// LastByte returns the terminating byte of the ID, with the number of upper
// bits that it uses (between 1 and 8, and 0 if the ID is empty). The remaining
// unused lower bits are always unset.
func (n NodeID2) LastByte() (byte, uint8) {
	return n.last, n.bits
}

// Prefix returns the prefix of NodeID2 with the given number of bits.
func (n NodeID2) Prefix(bits uint) NodeID2 {
	// Note: This code is very similar to NewNodeID2, and it's tempting to return
	// NewNodeID2(n.path, bits). But there is a difference: NewNodeID2 expects
	// all the bytes to be in the path string, while here the last byte is not.
	if bits == 0 {
		return NodeID2{}
	} else if mx := n.BitLen(); bits > mx {
		panic(fmt.Sprintf("Prefix: bits %d > %d", bits, mx))
	}
	bytes, tailBits := split(bits)
	last := n.last
	if bytes != uint(len(n.path)) {
		last = n.path[bytes]
	}
	return newMaskedNodeID2(n.path[:bytes], last, tailBits)
}

// Sibling returns the NodeID2 of the nodes's sibling in a binary tree, i.e.
// the ID of the parent node's other child. If the node is the root then the
// returned ID is the same.
func (n NodeID2) Sibling() NodeID2 {
	last := n.last ^ byte(1<<(8-n.bits))
	return NodeID2{path: n.path, last: last, bits: n.bits}
}

// String returns a human-readable bit string.
func (n NodeID2) String() string {
	if n.BitLen() == 0 {
		return "[]"
	}
	path := fmt.Sprintf("%08b", []byte(n.path))
	path = path[1 : len(path)-1] // Trim the brackets.
	if len(path) > 0 {
		path += " "
	}
	return fmt.Sprintf("[%s%0*b]", path, n.bits, n.last>>(8-n.bits))
}

// split returns the decomposition of a NodeID2 with the given number of bits.
// The first int returned is the number of full bytes stored in the dynamically
// allocated part. The second one is the number of bits in the tail byte.
func split(bits uint) (bytes uint, tailBits uint8) {
	return (bits - 1) / 8, uint8(1 + (bits-1)%8)
}

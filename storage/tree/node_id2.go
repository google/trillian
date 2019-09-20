// Copyright 2019 Google Inc. All Rights Reserved.
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

// NodeID2 is a faster NodeID that does zero memory allocations.
//
// TODO(pavelkalinnikov): Rename to NodeID and document it properly when the
// code has migrated.
type NodeID2 struct {
	path string
	last byte
	bits byte
}

// NewNodeID2 creates a NodeID2 from the given path bytes truncated to the
// specified number of bits if necessary. Panics if the number of bits is
// negative, or is more than the byte string contains.
func NewNodeID2(path string, bits int) NodeID2 {
	if bits == 0 {
		return NodeID2{}
	} else if mx := len(path) * 8; bits > mx {
		panic(fmt.Sprintf("NewNodeID2: bits %d > %d", bits, mx))
	}
	bytes, tail, mask := decompose(bits)
	last := path[bytes] & mask
	return NodeID2{path: path[:bytes], last: last, bits: byte(tail)}
}

// BitLen returns the length of the NodeID2 in bits.
func (n NodeID2) BitLen() int {
	return len(n.path)*8 + int(n.bits)
}

// Prefix returns the prefix of NodeID2 with the given number of bits.
func (n NodeID2) Prefix(bits int) NodeID2 {
	if bits == 0 {
		return NodeID2{}
	} else if mx := n.BitLen(); bits > mx {
		panic(fmt.Sprintf("Prefix: bits %d > %d", bits, mx))
	}
	last := n.last
	bytes, tail, mask := decompose(bits)
	if bytes != len(n.path) {
		last = n.path[bytes]
	}
	last &= mask
	return NodeID2{path: n.path[:bytes], last: last, bits: byte(tail)}
}

// Suffix returns the suffix of NodeID2 after the given number of bits.
func (n NodeID2) Suffix(bits int) NodeID2 {
	if mx := n.BitLen(); bits > mx {
		panic(fmt.Sprintf("Suffix: bits %d > %d", bits, mx))
	} else if bits == mx {
		return NodeID2{}
	} else if bits%8 == 0 {
		return NewNodeID2(n.path[bits/8:], mx-bits)
	}
	// TODO(pavelkalinnikov): Support arbitrary lengths.
	panic("Suffix: only multiples of 8 are supported")
}

// PrefixBytes returns a prefix of bytes*8 bits, as bytes. Must always be
// called with a prefix shorter than BitLen().
func (n NodeID2) PrefixBytes(bytes int) string {
	return n.path[:bytes]
}

// Sibling returns the NodeID2 of the nodes's sibling in a binary tree. If the
// node is the root then the returned ID is the same.
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

// decompose returns decomposition of a NodeID2 with the given number of bits.
//
// The first int of the returned triple is the number of full bytes stored in
// the dynamically allocated part. The second one is the number of bits in the
// tail byte (between 1 and 8). The third value is a mask with the
// corresponding number of higher bits set.
func decompose(bits int) (int, int, byte) {
	bytes := (bits - 1) / 8
	tailBits := 1 + (bits-1)%8
	mask := ^byte(1<<(8-tailBits) - 1)
	return bytes, tailBits, mask
}

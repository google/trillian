// Copyright 2017 Google Inc. All Rights Reserved.
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

package node

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/google/trillian/storage/storagepb"
)

const (
	// MaxLogDepth is the maximum number of levels a log is allowed to have.
	// This corresponds to the use of int64 to indicate leaf indexes.
	MaxLogDepth = 64
)

//
// Transition Plan
//

//
// Differences between Log and Map paths
// - Both are constant sized byte slices.
//
// - Log paths grow in significance from LSB to MSB.
//   max - depth is the height of the node. Bits to the right should be set to 0.
//
// - Map paths grow in signifiance from MSB to LSB.
//   depth indicates the number of bits to pay attention to.

// Node represents a location in a merkle tree, formed by a path and a depth.
// The root of a tree is indicated by depth 0.
// Leaves are stored at depth == len(path)*8
type Node struct {
	// Number of significant bits in path, starting from MSB.
	// Depth 0 means the root node, a path "" of length zero bits.
	depth int

	// Path is left-aligned. Nodes with depths that are not multiples of 8
	// will have unused bits in the least significant bit positions.
	path []byte
}

// New returns a node defined by a full path.
// For a map, this is a leaf node.
// For a log, this is also a leaf node.
func New(index []byte) *Node {
	return &Node{
		path:  index,
		depth: len(index) * 8,
	}
}

// NewFromRaw returns a new Node with exactly the params
// len(path)*8 should >= depth
func NewFromRaw(depth int, path []byte) *Node {
	return &Node{
		path:  path,
		depth: depth,
	}
}

// NewFromSubtree returns the Node for a location inside a subtree.
// subDepth is the number of levels down from the subtreeroot
// index is the horizontal index at that level, within the subtree.
func NewFromSubtree(st *storagepb.SubtreeProto, subDepth int, index int64, totalDepth int) *Node {
	if got, want := totalDepth%8, 0; got != want || got < want {
		panic(fmt.Sprintf("storage NewNodeFromPrefix(): totalDepth mod 8: %v, want %v", got, want))
	}

	// Put prefix in the MSB bits of path.
	path := make([]byte, totalDepth/8)
	copy(path, st.Prefix)

	// Convert index into absolute coordinates for subtree.
	subHeight := int(st.Depth) - subDepth
	subIndex := index << uint(subHeight) // index is the horizontal index at the given height.

	// Copy subDepth/8 bytes of subIndex into path.
	subPath := new(bytes.Buffer)
	binary.Write(subPath, binary.BigEndian, uint64(subIndex))
	unusedHighBytes := 64/8 - subDepth/8
	copy(path[len(st.Prefix):], subPath.Bytes()[unusedHighBytes:])

	return &Node{
		path:  path,
		depth: len(st.Prefix)*8 + subDepth,
	}
}

// NewFromSubtreeBig returns a Node given by a prefix and a subtree index.
// depth is the number of significant bits in subIndex, counting from the MSB.
// subIndex is the path from the root of the subtree to the desired node, and continuing down to the bottom of the subtree.
// subIndex = horizontal index << height.
func NewFromSubtreeBig(prefix []byte, depth int, subIndex *big.Int, totalDepth int) *Node {
	// Put prefix in the MSB bits of path.
	path := make([]byte, totalDepth/8)
	copy(path, prefix)

	// Copy subIndex into path.
	copy(path[len(prefix):], subIndex.Bytes())

	return &Node{
		path:  path,
		depth: len(prefix)*8 + depth,
	}
}

// NewFromBig returns a NodeID of a big.Int with no prefix.
// index contains the path's least significant bits.
// depth indicates the number of bits from the most significant bit to treat as part of the path.
func NewFromBig(depth int, index *big.Int, totalDepth int) *Node {
	if got, want := totalDepth%8, 0; got != want || got < want {
		panic(fmt.Sprintf("storage NewNodeFromBitInt(): totalDepth mod 8: %v, want %v", got, want))
	}

	// Put index in the LSB bits of path.
	path := make([]byte, totalDepth/8)
	unusedHighBytes := len(path) - len(index.Bytes())
	copy(path[unusedHighBytes:], index.Bytes())

	// TODO(gdbelvin): consider masking off insignificant bits past depth.

	return &Node{
		path:  path,
		depth: depth,
	}
}

// NewFromTreeCoords returns a node for a particular position in the tree.
// height is the number of levels above the leaves.  0 = leaves.
// index is the horizontal index into the tree at level depth, so the returned
// NodeID will be zero padded on the right by height places.
func NewFromTreeCoords(height int, index int64) *Node {
	absIndex := index << uint(height)
	b := new(bytes.Buffer)
	binary.Write(b, binary.BigEndian, uint64(absIndex))

	return &Node{
		path:  b.Bytes(),
		depth: MaxLogDepth - height,
	}
}

// Copy returns a copy of Node.
func (n *Node) Copy() *Node {
	p := make([]byte, len(n.path))
	copy(p, n.path)
	return &Node{
		path:  p,
		depth: n.depth,
	}
}

// Equal returns true iff paths are byte for byte equal (including unused bytes)
// and they have equal depths.
func Equal(a, b *Node) bool {
	return a.depth == b.depth && bytes.Equal(a.path, b.path)
}

// CoordString returns a string of the format:
// [d:<levels from leaves>, i:<horizontal index at level>]
// CoordString only parses the first 64 bits of path.
func (n *Node) CoordString() string {
	// Interpret path as an int64.
	var index uint64
	b := bytes.NewBuffer(n.path)
	binary.Read(b, binary.BigEndian, &index)

	height := uint(n.PathBits() - n.depth)
	hindex := index >> height

	return fmt.Sprintf("[d:%d, i:%d]", height, hindex)
}

// SubtreeKey returns a string that represents the path within a subtree.
// This is a base64 encoding of the following format:
// [ 1 byte for depth || path bytes ]
// This format MUST not change or on-disk trees will be unreadable.
func (n *Node) SubtreeKey() string {
	r := make([]byte, 1, 1+(len(n.path)))
	r[0] = byte(n.depth)
	r = append(r, n.path...)
	return base64.StdEncoding.EncodeToString(r)
}

// PathBits returns the maximum number of bits in path, including ignored bits.
// PathBits returns multiples of 8.
// For maps, this is the height of the full tree.
// For logs, this is the height of the current tree. XXX: what does that mean?
func (n *Node) PathBits() int {
	return len(n.path) * 8
}

// LeftBit returns the ith bit from MSB.
func (n *Node) LeftBit(i int) uint {
	panic("unimplemented")
}

// RightBit returns the ith bit from LSB.
// eg. RightBit(0x80000000, 31) -> 1
func (n *Node) RightBit(i int) uint {
	// TODO: assert i < PathBits()?
	bIndex := (n.PathBits() - i - 1) / 8
	return uint((n.path[bIndex] >> uint(i%8)) & 0x01)
}

// FlipRightBit flips the i'th bit from the right.
func (n *Node) FlipRightBit(i int) *Node {
	bIndex := (n.PathBits() - i - 1) / 8
	n.path[bIndex] ^= 1 << uint(i%8)
	return n
}

// leftmask contains bitmasks indexed such that the left x bits are set. It is
// indexed by byte position from 0-7 0 is special cased to 0xFF since 8 mod 8
// is 0. leftmask is only used to mask the last byte.
var leftmask = [8]byte{0xFF, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFC, 0xFE}

// MaskLeft keeps depth bits from MSB and zeros everything else.
// It also sets depth to depth.
func (n *Node) MaskLeft(depth int) *Node {
	r := make([]byte, len(n.path))
	if depth > 0 {
		// Copy the first depthBytes.
		depthBytes := (depth + 7) >> 3
		copy(r, n.path[:depthBytes])
		// Mask off unwanted bits in the last byte.
		r[depthBytes-1] = r[depthBytes-1] & leftmask[depth%8]
	}

	if depth < n.depth {
		n.depth = depth
	}
	n.path = r
	return n
}

// Siblings returns the neighbors of this node, starting in order from LSB to MSB.
func (n *Node) Siblings() []*Node {
	nbrs := make([]*Node, n.depth)
	for height := range nbrs {
		depth := n.PathBits() - height
		nbrs[height] = n.Copy().FlipRightBit(height).MaskLeft(depth)
	}
	return nbrs
}

// trimLeft removes unused bytes from the LSB portion of path.
func (n *Node) trimRight() *Node {
	bytesNeeded := (n.depth + 7) / 8
	n.path = n.path[:bytesNeeded]
	return n
}

// cutLeft removes the first i bytes from MSB
func (n *Node) cutLeft(i int) *Node {
	n.path = n.path[i:]
	n.depth -= i * 8
	return n
}

// Split splits a Node into a prefix Node at prefixBytes and a subtree path
// continaing the following subDepth bits.
func (n *Node) Split(prefixBytes, subDepth int) (*Node, *Node) {
	prefix := n.Copy().MaskLeft(prefixBytes * 8).trimRight()
	subPath := n.Copy().cutLeft(prefixBytes).MaskLeft(subDepth).trimRight()

	return prefix, subPath
}

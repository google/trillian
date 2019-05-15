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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/bits"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian/storage/storagepb"
)

const hexChars = "0123456789abcdef"

// Node represents a single node in a Merkle tree.
type Node struct {
	NodeID       NodeID
	Hash         []byte
	NodeRevision int64
}

// NodeID uniquely identifies a Node within a MerkleTree.
// Reading paths right to left is the natural order when traversing from
// leaves towards the root. However, for internal nodes the rightmost bits
// of the IDs are not aligned on a byte boundary so care must be taken.
//
// NodeIDs as presented to the storage layer will always have a multiple
// of 8 bits as they identify the root of a 256 element subtree.
//
// Note that some of the APIs count bits with 0 being the rightmost
// in the entire path array when some of these might not be significant.
//
// See the types.go file that defines this type for more detailed information
// and docs/storage for how they are used in the on-disk representation of
// Merkle trees.
type NodeID struct {
	// Path is effectively a BigEndian bit set, with the MSB of Path[0]
	// identifying the root child, and successive bits identifying the lower
	// level children down to the leaf.
	Path []byte
	// PrefixLenBits is the number of significant bits in Path, which are
	// considered part of this NodeID.
	//
	// e.g. if Path contains two bytes, and PrefixLenBits is 9, then the 8 bits
	// in Path[0] are included, along with the MSB of Path[1]. The remaining
	// 7 bits are not significant.
	PrefixLenBits int
}

// Some Additional NodeID examples and Documentation.
//
// Consider a hypothetical case of a tree of size 256 and max path length 8.
// The IDs within this tree will at most have 8 significant bits. To specify
// the leaf with index 95 requires 8 bits to match the tree height so the
// Path array would be: [0x5f] with PrefixLength 8. Moving up the tree two
// levels from this node the depth would be 2 and index 23. This gives a Path
// array of [0x5c] and PrefixLength is 6, which means the lowest 2 bits are not
// significant. In binary 0x5c is: 0b01011100 and 23 decimal is: 0b000010111
// so the index has been shifted left by 2 places in the NodeID. Reading
// from left to right after 6 bits the node has been reached. The remaining
// two bits therefore don't matter.
//
// Larger tree sizes as used by Logs and Maps work in exactly the same
// way but with the Path taking up multiple bytes. For example if we move
// to a tree size of up to 65535 there will be two Path bytes. For a leaf
// in this tree at depth = 0, index = 19 the resulting NodeID has a
// Path array of [0x00, 0x13] and PrefixLength 16. To see that it's the
// left bits that are significant consider depth = 2, index = 16383 in this
// tree (the rightmost node in a complete tree at this level). The resulting
// NodeID has Path [0xff, 0xfc] and PrefixLength is 14. When processing this
// NodeID to move up the tree the lowest 2 bits of Path[1] should be ignored as
// it only takes 14 bits to reach the root of the tree from level 2 in a 16
// levels deep tree. Also, testing Bit(0) on this NodeID would not be valid as
// this is the LSB of Path[1] and it's not one of the significant bits.
//
// When dealing with storage the ID can be split into a prefix and suffix.
// All nodes within the same storage subtree have the same prefix and form
// the subtree ID. The suffix identifies the internal node within the subtree.
// They are both paths in the entire Merkle tree. In this case the prefix will
// always be a multiple of 8 bits.

// PathLenBits returns 8 * len(path).
func (n NodeID) PathLenBits() int {
	return len(n.Path) * 8
}

// bytesForBits returns the number of bytes required to store numBits bits.
func bytesForBits(numBits int) int {
	return (numBits + 7) >> 3
}

// NewNodeIDFromHash creates a new NodeID for the given Hash.
func NewNodeIDFromHash(h []byte) NodeID {
	return NodeID{
		Path:          h,
		PrefixLenBits: len(h) * 8,
	}
}

// NewEmptyNodeID creates a new zero-length NodeID with sufficient underlying
// capacity to store a maximum of maxLenBits.
func NewEmptyNodeID(maxLenBits int) NodeID {
	if got, want := maxLenBits%8, 0; got != want {
		panic(fmt.Sprintf("storage: NewEmptyNodeID() maxLenBits mod 8: %v, want %v", got, want))
	}
	return NodeID{
		Path:          make([]byte, maxLenBits/8),
		PrefixLenBits: 0,
	}
}

// NewNodeIDFromPrefix returns a nodeID for a particular node within a subtree.
// Prefix is the prefix of the subtree.
// depth is the depth of index from the root of the subtree.
// index is the horizontal location of the subtree leaf.
// subDepth is the total number of levels in the subtree.
// totalDepth is the number of levels in the whole tree.
func NewNodeIDFromPrefix(prefix []byte, depth int, index int64, subDepth, totalDepth int) NodeID {
	if got, want := totalDepth%8, 0; got != want || got < want {
		panic(fmt.Sprintf("storage NewNodeFromPrefix(): totalDepth mod 8: %v, want %v", got, want))
	}
	if got, want := subDepth%8, 0; got != want || got < want {
		panic(fmt.Sprintf("storage NewNodeFromPrefix(): subDepth mod 8: %v, want %v", got, want))
	}
	if got, want := depth, 0; got < want {
		panic(fmt.Sprintf("storage NewNodeFromPrefix(): depth: %v, want >= %v", got, want))
	}

	// Put prefix in the left (most significant) bits of path.
	path := make([]byte, totalDepth/8)
	copy(path, prefix)

	// Convert index into absolute coordinates for subtree.
	height := subDepth - depth
	subIndex := index << uint(height) // index is the horizontal index at the given height.

	// Copy subDepth/8 bytes of subIndex into path.
	subPath := new(bytes.Buffer)
	// Write() on a Buffer never errors. It panics internally if we run out of RAM.
	binary.Write(subPath, binary.BigEndian, uint64(subIndex))
	unusedHighBytes := 64/8 - subDepth/8
	copy(path[len(prefix):], subPath.Bytes()[unusedHighBytes:])

	return NodeID{
		Path:          path,
		PrefixLenBits: len(prefix)*8 + depth,
	}
}

// NewNodeIDFromBigInt returns a NodeID of a big.Int with no prefix.
// index contains the path's least significant bits.
// depth indicates the number of bits from the most significant bit to treat as part of the path.
func NewNodeIDFromBigInt(depth int, index *big.Int, totalDepth int) NodeID {
	if got, want := totalDepth%8, 0; got != want || got < want {
		panic(fmt.Sprintf("storage NewNodeFromBitInt(): totalDepth mod 8: %v, want %v", got, want))
	}

	// Put index in the LSB bits of path.
	path := make([]byte, totalDepth/8)
	unusedHighBytes := len(path) - len(index.Bytes())
	copy(path[unusedHighBytes:], index.Bytes())

	// TODO(gdbelvin): consider masking off insignificant bits past depth.
	if glog.V(5) {
		glog.Infof("NewNodeIDFromBigInt(%v, %x, %v): %v, %x",
			depth, index.Bytes(), totalDepth, depth, path)
	}

	return NodeID{
		Path:          path,
		PrefixLenBits: depth,
	}
}

// BigInt returns the big.Int for this node.
func (n NodeID) BigInt() *big.Int {
	return new(big.Int).SetBytes(n.Path)
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
// This method is used exclusively by the Log, and, since the Log model grows upwards from the
// leaves, we modify the provided coords accordingly.
//
// depth is the Merkle tree level: 0 = leaves, and increases upwards towards the root.
//
// index is the horizontal index into the tree at level depth, so the returned
// NodeID will be zero padded on the right by depth places.
func NewNodeIDForTreeCoords(depth int64, index int64, maxPathBits int) (NodeID, error) {
	bl := bits.Len64(uint64(index))
	if index < 0 || depth < 0 ||
		bl > int(maxPathBits-int(depth)) ||
		maxPathBits%8 != 0 {
		return NodeID{}, fmt.Errorf("depth/index combination out of range: depth=%d index=%d maxPathBits=%v", depth, index, maxPathBits)
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
// Note that the bit index 0 is the right most valid bit in the path.
func (n *NodeID) SetBit(i int, b uint) {
	bIndex := (n.PathLenBits() - i - 1) / 8
	if b == 0 {
		n.Path[bIndex] &= ^(1 << uint(i%8))
	} else {
		n.Path[bIndex] |= (1 << uint(i%8))
	}
}

// Bit returns 1 if the zero indexed ith bit from the right (of the whole path
// array, not just the significant portion) is true, and false otherwise.
func (n *NodeID) Bit(i int) uint {
	if got, want := i, n.PathLenBits()-1; got > want {
		panic(fmt.Sprintf("storage: Bit(%v) > (PathLenBits() -1): %v", got, want))
	}
	bIndex := (n.PathLenBits() - i - 1) / 8
	return uint((n.Path[bIndex] >> uint(i%8)) & 0x01)
}

// String returns a string representation of the binary value of the NodeID.
// The left-most bit is the MSB (i.e. nearer the root of the tree). The
// length of the returned string will always be the same as the prefix length
// of the node. For a string ID to use as a map key see AsKey().
func (n *NodeID) String() string {
	var r bytes.Buffer
	limit := n.PathLenBits() - n.PrefixLenBits
	for i := n.PathLenBits() - 1; i >= limit; i-- {
		r.WriteRune(rune('0' + n.Bit(i)))
	}
	return r.String()
}

// AsKey returns a string identifier for this NodeID suitable for
// short term use e.g. as a Map key. It is more efficient to use this than
// String() as it's not constrained to return a binary string.
func (n *NodeID) AsKey() string {
	var b strings.Builder
	fullBytes := n.PrefixLenBits / 8
	bitsLeft := n.PrefixLenBits % 8

	// Note: all Builder write methods are documented never to return errors.
	// Write the length first.
	b.WriteString(strconv.Itoa(n.PrefixLenBits))
	b.WriteRune(':')
	// We can do all the full bytes in one go.
	if fullBytes > 0 {
		buf := make([]byte, hex.EncodedLen(fullBytes))
		hex.Encode(buf, n.Path[:fullBytes])
		b.Write(buf)
	}
	// If there's bits left over write them out.
	if bitsLeft > 0 {
		bits := n.Path[fullBytes] & leftmask[bitsLeft]
		b.WriteByte(hexChars[bits>>4])
		b.WriteByte(hexChars[bits&0xf])
	}

	return b.String()
}

// CoordString returns a string representation assuming that the NodeID represents a
// tree coordinate. Using this on a NodeID for a sparse Merkle tree will give incorrect
// results. Intended for debugging purposes, the format could change.
func (n *NodeID) CoordString() string {
	d := uint64(n.PathLenBits() - n.PrefixLenBits)
	i := uint64(0)
	for _, p := range n.Path {
		i = (i << uint64(8)) + uint64(p)
	}

	return fmt.Sprintf("[d:%d, i:%d]", d, i>>d)
}

// Copy returns a duplicate of NodeID
func (n *NodeID) Copy() *NodeID {
	p := make([]byte, len(n.Path))
	copy(p, n.Path)
	return &NodeID{
		Path:          p,
		PrefixLenBits: n.PrefixLenBits,
	}
}

// FlipRightBit flips the ith bit from LSB
func (n *NodeID) FlipRightBit(i int) *NodeID {
	n.SetBit(i, n.Bit(i)^1)
	return n
}

// leftmask contains bitmasks indexed such that the left x bits are set. It is
// indexed by byte position from 0-7 0 is special cased to 0xFF since 8 mod 8
// is 0. leftmask is only used to mask the last byte.
var leftmask = [8]byte{0xFF, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFC, 0xFE}

// MaskLeft returns NodeID with only the left n bits set
func (n *NodeID) MaskLeft(depth int) *NodeID {
	r := make([]byte, len(n.Path))
	if depth > 0 {
		// Copy the first depthBytes.
		depthBytes := bytesForBits(depth)
		copy(r, n.Path[:depthBytes])
		// Mask off unwanted bits in the last byte.
		r[depthBytes-1] = r[depthBytes-1] & leftmask[depth%8]
	}
	if depth < n.PrefixLenBits {
		n.PrefixLenBits = depth
	}
	n.Path = r
	return n
}

// Neighbor returns the same node with the bit at PrefixLenBits flipped.
// In terms of a tree traversal, this is the parent node's other child node
// in the binary tree (often termed sibling node).
func (n *NodeID) Neighbor() *NodeID {
	height := n.PathLenBits() - n.PrefixLenBits
	n.FlipRightBit(height)
	return n
}

// Siblings returns the siblings of the given node.
// In terms of a tree traversal, this returns the Neighbour() of every node
// (including this one) on the path up to the root. The array is of length
// PrefixLenBits and is ordered such that the nodes closest to the leaves are
// earlier in the array.
// These nodes are the ones that would be required for a Merkle tree inclusion
// proof for this node.
func (n *NodeID) Siblings() []NodeID {
	sibs := make([]NodeID, n.PrefixLenBits)
	for height := range sibs {
		depth := n.PrefixLenBits - height
		sibs[height] = *(n.Copy().MaskLeft(depth).Neighbor())
	}
	return sibs
}

// NewNodeIDFromPrefixSuffix undoes Split() and returns the NodeID.
func NewNodeIDFromPrefixSuffix(prefix []byte, suffix *Suffix, maxPathBits int) NodeID {
	path := make([]byte, maxPathBits/8)
	copy(path, prefix)
	copy(path[len(prefix):], suffix.path)

	return NodeID{
		Path:          path,
		PrefixLenBits: len(prefix)*8 + int(suffix.bits),
	}
}

// Split splits a NodeID into a prefix and a suffix at prefixBytes.
func (n *NodeID) Split(prefixBytes, suffixBits int) ([]byte, *Suffix) {
	if n.PrefixLenBits == 0 {
		return []byte{}, EmptySuffix
	}
	a := make([]byte, len(n.Path))
	copy(a, n.Path)

	b := n.PrefixLenBits - prefixBytes*8
	if b > suffixBits {
		panic(fmt.Sprintf("storage Split: %x(n.PrefixLenBits: %v - prefixBytes: %v *8) > %v", n.Path, n.PrefixLenBits, prefixBytes, suffixBits))
	}
	if b <= 0 {
		panic(fmt.Sprintf("storage Split: %x(n.PrefixLenBits: %v - prefixBytes: %v *8) <= 0", n.Path, n.PrefixLenBits, prefixBytes))
	}
	suffixBytes := bytesForBits(b)
	maskIndex := (b - 1) / 8
	maskLowBits := (b-1)%8 + 1
	sfxPath := a[prefixBytes : prefixBytes+suffixBytes]
	sfxPath[maskIndex] &= ((0x01 << uint(maskLowBits)) - 1) << uint(8-maskLowBits)
	sfx := NewSuffix(byte(b), sfxPath)

	return a[:prefixBytes], sfx
}

// Equivalent return true iff the other represents the same path prefix as this NodeID.
func (n *NodeID) Equivalent(other NodeID) bool {
	// If they're different lengths they cannot represent the same path prefix.
	if n.PrefixLenBits != other.PrefixLenBits {
		return false
	}

	// The first depthBytes must be identical.
	depthBytes := n.PrefixLenBits / 8
	if !bytes.Equal(n.Path[:depthBytes], other.Path[:depthBytes]) {
		return false
	}

	// There may not be a leftover partial byte to compare.
	if n.PrefixLenBits%8 == 0 {
		return true
	}

	// Check the remaining bits after masking off unwanted bits in the last byte.
	mask := leftmask[n.PrefixLenBits%8]
	return (n.Path[depthBytes] & mask) == (other.Path[depthBytes] & mask)
}

// PopulateSubtreeFunc is a function which knows how to re-populate a subtree
// from just its leaf nodes.
type PopulateSubtreeFunc func(*storagepb.SubtreeProto) error

// PrepareSubtreeWriteFunc is a function that carries out any required tree type specific
// manipulation of a subtree before it's written to storage
type PrepareSubtreeWriteFunc func(*storagepb.SubtreeProto) error

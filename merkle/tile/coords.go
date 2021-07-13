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

package tile

import (
	"fmt"
	"math/bits"

	"github.com/golang/glog"
)

// MaxTreeHeight is the maximum height of trees whose coordinates can be
// dealt with; attempts to use larger trees will cause panic.
const MaxTreeHeight = 60

// MerkleCoords describes the position of a node in an append-only binary
// tree.  For nodes within a complete binary tree the position is encoded as
// level.index, with level counting up from 0 (leaves) and index counting in
// from the left of the tree consecutively:
//                    3.0 ...
//           2.0                2.1 ...
//       1.0      1.1      1.2      1.3 ...
//     0.0 0.1  0.2 0.3  0.4 0.5  0.6 0.7 ...
// as introduced in [TLOG].  Note that because the tree is append-only, a given
// node position of this form will always refer to the same set of leaves.
//
// This notation is extended to include pseudo-nodes for binary trees whose size
// is not a power of 2, recorded as the (single) incomplete node at a specific
// level.  The level indicates which sibling (complete) node would be paired
// with this one in the tree hashing scheme of [RFC6962].  Note that the
// incomplete node for a particular level is dependent on tree size, as recorded
// in the TreeSize field.  Pseudo-nodes are therefore notated like:
//   <level>.x@<treesize>
// or:
//   <level>.x@<treesize>=[<start>,<treesize>)
// to show the range of leaves covered.
//
// For example, a tree of size 5 has a pseudo-root 3.x@5 that covers the whole
// tree [0,5):
//
//                 3.x@5=[0,5)
//            2.0         |
//       1.0      1.1     |
//     0.0 0.1  0.2 0.3  0.4
//
// However, in a tree of size 13:
//                                    4.x@13=[0,13)
//                    3.0                              3.x@13=[8,13)
//           2.0                2.1               2.2         |
//       1.0      1.1      1.2      1.3       1.4     1.5     |
//     0.0 0.1  0.2 0.3  0.4 0.5  0.6 0.7   0.8 0.9 0.10 0.11 0.12
//
// node 3.x@13 covers the subrange [8,13).  Moreover, observe that 3.x@13
// combines two complete subtrees of different heights, 2.2 and 0.12.
type MerkleCoords struct {
	// Level indicates the height of a node in the tree, where
	// leaf nodes are level 0, parents of leaf nodes are level 1
	// and so on.
	Level uint64
	// Index runs from left to right, for nodes with TreeSize==0.
	Index uint64
	// If TreeSize is non-zero, this is a pseudo-node in a tree of this
	// size (and Index is ignored).
	TreeSize uint64
}

func (p MerkleCoords) String() string {
	if p.TreeSize > 0 {
		// Always show the covered range for pseudo-nodes, so it is easier
		// to figure out their substructure.
		l, r := p.LeafRange()
		return fmt.Sprintf("%d.x@%d=[%d,%d)", p.Level, p.TreeSize, l, r)
	}
	return fmt.Sprintf("%d.%d", p.Level, p.Index)
}

// IsPseudoNode indicates whether the coordinates describe a pseudo-node in an
// incomplete binary tree.
func (p MerkleCoords) IsPseudoNode() bool {
	return p.TreeSize > 0
}

// LeafRange returns the [left, right) range of leaf indices encompassed
// by the given node.
func (p MerkleCoords) LeafRange() (uint64, uint64) {
	if p.Level >= MaxTreeHeight {
		panic(fmt.Sprintf("can't cope with nodes of level %d", p.Level))
	}
	subtreeSize := uint64(1) << p.Level
	if p.TreeSize > 0 {
		// Leaves under level L have indices starting on a subtreeSize=2^L boundary.
		// Find the largest multiple of subtreeSize that fits fully within the tree;
		// this pseudo-node covers from after that up to the tree size.
		n := (p.TreeSize / subtreeSize)
		return n * subtreeSize, p.TreeSize
	}
	// For a node in a complete binary tree, just count in by the subtree size.
	// Sanity check examples:
	//    Level Index  subtreeSize    Range
	//      0     0        1      [0,1)
	//      0     1        1      [1,2)
	//      0     2        1      [2,3)
	//      0     n        1      [n,n+1)
	//
	//      1     0        2      [0,2)
	//      1     1        2      [2,4)
	//      1     n        2      [2n,2(n+1))
	//
	//      2     0        4      [0,4)
	//      2     1        4      [4,8)
	//      ...

	// LeafRange is [p.Index * subtreeSize, (p.Index + 1) * subtreeSize), but
	// as subtreeSize = 1 << L, this is [p.Index << L, (p.Index+1) << L).
	// Check for overflow first:
	if (uint64(bits.Len64(p.Index+1)) + p.Level) > 63 {
		panic(fmt.Sprintf("overflowing leaf range: [%d << %d, %d << %d) overflows 64b", p.Index, p.Level, p.Index+1, p.Level))
	}
	return p.Index << p.Level, (p.Index + 1) << p.Level
}

// LeftChild returns the MerkleCoords for the left child of the given node.  This function
// panics if supplied with a leaf node.
func (p MerkleCoords) LeftChild() MerkleCoords {
	if p.Level == 0 {
		panic(fmt.Sprintf("left child of leaf node %s requested", p))
	}
	if p.TreeSize == 0 {
		return MerkleCoords{
			Level: p.Level - 1, // one level down
			Index: 2 * p.Index,
		}
	}
	// "k [is] the largest power of two smaller than [treeSize]".
	// "MTH(D[n]) = SHA-256(0x01 || MTH(D[0:k]) || MTH(D[k:n])),"
	// Need a range that starts at zero for this.
	l, r := p.LeafRange()
	k, _ := largestPowerOfTwoBelow(r - l)
	return rootNodeForRange(0, k).moveRightBy(l)
}

// RightChild returns the MerkleCoords for the right child of the given node.  This function
// panics if supplied with a leaf node.
func (p MerkleCoords) RightChild() MerkleCoords {
	if p.Level == 0 {
		panic(fmt.Sprintf("right child of leaf node %s requested", p))
	}
	if p.TreeSize == 0 {
		return MerkleCoords{
			Level: p.Level - 1, // one level down
			Index: 2*p.Index + 1,
		}
	}

	// "k [is] the largest power of two smaller than [treeSize]".
	// "MTH(D[n]) = SHA-256(0x01 || MTH(D[0:k]) || MTH(D[k:n])),"
	// Need a range that starts at zero for this.
	l, r := p.LeafRange()
	k, _ := largestPowerOfTwoBelow(r - l)
	return rootNodeForRange(k, r-l).moveRightBy(l)
}

// Contains reports whether a node is a descendent of another node.
func (p MerkleCoords) Contains(candidate MerkleCoords) bool {
	innerL, innerR := candidate.LeafRange()
	outerL, outerR := p.LeafRange()
	return ((innerL >= outerL && innerL < outerR) && ((innerR-1) >= outerL && (innerR-1) < outerR))
}

// rootNodeForRange returns the coordinates of a node that exactly covers the
// leaf range [from, to).  The left side of the leaf range must either be zero
// or fall on a power-of-two boundary that is larger than the requested
// range. The right side of the range is assumed to be the overall tree size if
// the range does not encompass a complete subtree.
func rootNodeForRange(from, to uint64) MerkleCoords {
	if to <= from {
		panic(fmt.Sprintf("inverted range [%d, %d)", from, to))
	}
	if bits.OnesCount64(from) > 1 {
		panic(fmt.Sprintf("left of range [%d, %d) not a power of two", from, to))
	}

	if from == 0 {
		// Sanity check:            (size-1)  L=bitlen(treeSize-1)  1<<L
		//   0.0    covers [0,1)        0b0        0                 1
		//   1.0    covers [0,2)        0b1        1                 2
		//   2.x@3  covers [0,3)       0b10        2                 4
		//   2.0    covers [0,4)       0b11        2                 4
		//   3.x@5  covers [0,5)      0b100        3                 8
		//   3.x@6  covers [0,6)      0b101        3                 8
		//   3.x@7  covers [0,7)      0b110        3                 8
		//   3.0    covers [0,8)      0b111        3                 8
		//   4.x@9  covers [0,9)     0b1000        4                16
		//   4.x@10 covers [0,10)    0b1001        4                16
		l := uint64(bits.Len64(to - 1))
		if to == (uint64(1) << l) {
			return MerkleCoords{Level: l, Index: 0}
		}
		return MerkleCoords{Level: l, TreeSize: to}
	}

	// The left boundary is a power-of-two, but this needs to be bigger than
	// the size of the requested range.  (If it were not, [from, 2*from) would
	// be contained within [from, to), and we could have combined [0,from) with
	// [from, 2*from) to start at a bigger left hand complete subtree.)
	leafCount := to - from
	if leafCount > from {
		panic(fmt.Sprintf("bigger subtree at right for range [%d, %d)", from, to))
	}
	return rootNodeForRange(0, leafCount).moveRightBy(from)
}

// moveRightBy translates coordinates rightwards by a power of two.
func (p MerkleCoords) moveRightBy(k uint64) MerkleCoords {
	if p.TreeSize > 0 {
		// A pseudo-node in a tree that is translated right by
		// prepending a complete tree stays a pseudo-node at the same
		// level, but is now relative to a bigger tree.
		return MerkleCoords{
			Level:    p.Level,
			TreeSize: p.TreeSize + k,
		}
	}
	// Level 0 nodes (leaves) need to be moved along by k.
	// Level 1 nodes need to be moved along by k/2.
	// ..
	// Level L nodes need to be moved along by k/(2^L).
	return MerkleCoords{
		Level: p.Level,
		Index: p.Index + (k / (uint64(1) << p.Level)),
	}
}

// largestPowerOfTwoBelow returns k, the largest power of two less than
// size (so k < size <= 2k. It also returns the height of a tree of that
// size.
func largestPowerOfTwoBelow(size uint64) (uint64, uint64) {
	// Sanity check table:
	//  treeSize  height  2^(height-2)  k      (treeSize-1)  bitlen(treeSize-1)
	//     0        -          -        -           -              -
	//     1        1          -        -          0b0             0
	//     2        2          1        1          0b1             1
	//     3        3          2        2         0b10             2
	//     4        3          2        2         0b11             2
	//     5        4          4        4        0b100             3
	//     6        4          4        4        0b101             3
	//     7        4          4        4        0b110             3
	//     8        4          4        4        0b111             3
	//     9        5          8        8       0b1000             4
	//    ..       ..                  ..
	//    16        5          8        8       0b1111             4
	//    17        6         16       16      0b10000             5
	height := uint64(bits.Len64(size-1)) + 1
	k := uint64(1) << (height - 2)
	return k, height
}

// InclusionPath returns the inclusion path for a leaf at the given index in a
// (possibly incomplete) Merkle tree of the given size, starting from the
// required sibling leaf and ending with a child of the root node, following the
// algorithm from [RFC6962] section 2.1.1.
func InclusionPath(idx, treeSize uint64) []MerkleCoords {
	if treeSize < 2 {
		// If there's no leaf or a single leaf, no inclusion path is needed.
		glog.V(4).Infof("PATH(%d, D[%d]) = nil", idx, treeSize)
		return []MerkleCoords{}
	}
	if idx >= treeSize {
		return nil // OUTATREE
	}

	// "k [is] the largest power of two smaller than [treeSize]".
	k, height := largestPowerOfTwoBelow(treeSize)
	if idx < k {
		// Leaf is in left-most complete tree.
		// "PATH(m, D[n]) = PATH(m, D[0:k]) : MTH(D[k:n]) for m < k"
		glog.V(4).Infof("PATH(%d, D[%d]) = PATH(%d, D[%d]) : MTH(D[%d,%d))", idx, treeSize, idx, k, k, treeSize)
		path := InclusionPath(idx, k)
		fullPath := make([]MerkleCoords, len(path)+1)
		copy(fullPath, path)

		fullPath[len(path)] = rootNodeForRange(k, treeSize)
		glog.V(4).Infof("MTH(D[%d,%d)) = %v", k, treeSize, fullPath[len(path)])
		return fullPath
	}

	// Leaf is inside a subtree not at the left.  Treat the whole RHS of the
	// current tree as a new tree and start with a path in that.
	// "PATH(m, D[n]) = PATH(m - k, D[k:n]) : MTH(D[0:k]) for m >= k,"
	glog.V(4).Infof("PATH(%d, D[%d]) = PATH(%d = %d - %d, D[%d:%d]) : MTH(D[%d,%d))", idx, treeSize, idx-k, idx, k, k, treeSize, 0, k)
	path := InclusionPath(idx-k, treeSize-k)
	fullPath := make([]MerkleCoords, len(path)+1)
	for i, c := range path {
		fullPath[i] = c.moveRightBy(k)
	}
	// Add on the root of the left-most complete subtree.
	// MTH(D[0:k]) is node L.0 for L such that 2^L = k.
	fullPath[len(path)] = MerkleCoords{Level: height - 2, Index: 0}
	glog.V(4).Infof("MTH(D[%d,%d)) = %v", 0, k, fullPath[len(path)])
	return fullPath
}

// ConsistencyPath returns the path of nodes for a consistency proof between
// tree sizes, following the algorithm from [RFC6962] section 2.1.2.
func ConsistencyPath(fromSize, toSize uint64) []MerkleCoords {
	if fromSize >= toSize {
		return nil
	}
	glog.V(4).Infof("PROOF(%d, D[%d]) = SUBPROOF(%d, D[%d], true)", fromSize, toSize, fromSize, toSize)
	return consistencyPath(fromSize, toSize, true)
}

func consistencyPath(fromSize, toSize uint64, isOriginalFrom bool) []MerkleCoords {
	if fromSize == toSize {
		if isOriginalFrom {
			// "The subproof for m = n is empty if m is the value for which PROOF was
			// originally requested (meaning that the subtree Merkle Tree Hash
			// MTH(D[0:m]) is known)"
			glog.V(4).Infof("SUBPROOF(%d, D[%d], true) = nil", fromSize, toSize)
			return nil
		}
		// "The subproof for m = n is the Merkle Tree Hash committing inputs
		// D[0:m]; otherwise"
		coord := rootNodeForRange(0, fromSize)
		glog.V(4).Infof("SUBPROOF(%d, D[%d], false) = {MTH(D[%d])} = %v", fromSize, toSize, toSize, coord)
		return []MerkleCoords{coord}
	}

	// "For [fromSize] < [toSize], let k be the largest power of two smaller than [toSize]".
	k, _ := largestPowerOfTwoBelow(toSize)
	if fromSize <= k {
		// "If m <= k, the right subtree entries D[k:n] only exist in the current
		// tree.  We prove that the left subtree entries D[0:k] are consistent
		// and add a commitment to D[k:n]"
		glog.V(4).Infof("SUBPROOF(%d, D[%d], %v) = SUBPROOF(%d, D[0:%d], %v) : MTH(D[%d:%d])", fromSize, toSize, isOriginalFrom, fromSize, k, isOriginalFrom, k, toSize)
		path := consistencyPath(fromSize, k, isOriginalFrom)
		fullPath := make([]MerkleCoords, len(path)+1)
		copy(fullPath, path)

		fullPath[len(path)] = rootNodeForRange(k, toSize)
		glog.V(4).Infof("MTH(D[%d:%d]) = %v", k, toSize, fullPath[len(path)])
		return fullPath
	}

	// "If m > k, the left subtree entries D[0:k] are identical in both
	// trees.  We prove that the right subtree entries D[k:n] are consistent
	//  and add a commitment to D[0:k].
	glog.V(4).Infof("SUBPROOF(%d, D[%d], %v) = SUBPROOF(%d = %d - %d, D[%d:%d], false) : MTH(D[0:%d])", fromSize, toSize, isOriginalFrom, fromSize-k, fromSize, k, k, toSize, k)
	path := consistencyPath(fromSize-k, toSize-k, false)
	fullPath := make([]MerkleCoords, len(path)+1)
	for i, c := range path {
		fullPath[i] = c.moveRightBy(k)
	}
	fullPath[len(path)] = rootNodeForRange(0, k)
	glog.V(4).Infof("MTH(D[0:%d]) = %v", k, fullPath[len(path)])
	return fullPath
}

// CompleteSubtrees returns the list of complete subtrees associated with a
// Merkle tree of the given size.
func CompleteSubtrees(treeSize uint64) []MerkleCoords {
	if treeSize == 0 {
		return nil
	}
	var results []MerkleCoords
	for leafIdx := uint64(0); treeSize > 0; {
		len := uint64(bits.Len64(treeSize))
		// 2^(len-1) is the largest power of 2 that is <= treeSize
		subtreeSize := uint64(1) << (len - 1)
		results = append(results, MerkleCoords{Level: len - 1, Index: leafIdx / subtreeSize})

		leafIdx += subtreeSize
		treeSize -= subtreeSize
	}
	return results
}

// References:
//
// [CW] Crosby, S. and D. Wallach, "Efficient Data Structures for Tamper-Evident
// Logging", Proceedings of the 18th USENIX Security Symposium, Montreal, August
// 2009, http://static.usenix.org/event/sec09/ tech/full_papers/crosby.pdf.
//
// [RFC6962] B. Laurie, A. Langley, and E. Kasper, "Certificate Transparency", June
// 2013, RFC 6962.
//
// [TLOG] Cox, R, "Transparency Logs for Skeptical Clients", March 2019,
// https://research.swtch.com/tlog.

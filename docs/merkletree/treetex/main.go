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

// A binary to produce LaTeX documents representing Merkle trees.
// The generated document should be fed into xelatex, and the Forest package
// must be available.
//
// Usage: go run main.go | xelatex
// This should generate a PDF file called treetek.pdf containing a drawing of
// the tree.
//
package main

import (
	"flag"
	"fmt"
	"log"
	"math/bits"
	"strings"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
)

const (
	preamble = `
% Hash-tree
% Author: treetex
\documentclass[convert]{standalone}
\usepackage[dvipsnames]{xcolor}
\usepackage{forest}


\begin{document}

% Change colours here:
\definecolor{inclusion}{rgb}{1,0.5,0.5}
\definecolor{inclusion_ephemeral}{rgb}{1,0.7,0.7}
\definecolor{perfect}{rgb}{1,0.9,0.5}
\definecolor{target}{rgb}{0.5,0.5,0.9}
\definecolor{target_path}{rgb}{0.7,0.7,0.9}
\definecolor{mega}{rgb}{0.9,0.9,0.9}
\definecolor{range1}{rgb}{0.3,0.9,0.3}
\definecolor{range2}{rgb}{0.3,0.3,0.9}
\definecolor{range3}{rgb}{0.9,0.3,0.9}

\forestset{
	% This defines a new "edge" style for drawing the perfect subtrees.
	% Rather than simply drawing a line representing an edge, this draws a
	% triangle between the labelled anchors on the given nodes.
	% See "Anchors" section in the Forest manual for more details:
	%  http://mirrors.ibiblio.org/CTAN/graphics/pgf/contrib/forest/forest-doc.pdf
	perfect/.style={edge path={%
		\noexpand\path[fill=mega, \forestoption{edge}]
				(.parent first)--(!u.children)--(.parent last)--cycle
				\forestoption{edge label};
		}
	},
}
\begin{forest}
`

	postfix = `\end{forest}
\end{document}
`

	// Maximum number of ranges to allow.
	maxRanges = 3

	// maxLen is a suitably large maximum nodeID length for storage.NodeID.
	maxLen = 64
)

var (
	treeSize  = flag.Int64("tree_size", 23, "Size of tree to produce")
	inclusion = flag.Int64("inclusion", -1, "Leaf index to show inclusion proof")
	megaMode  = flag.Int64("megamode_threshold", 4, "Treat perfect trees larger than this many layers as a single entity")
	ranges    = flag.String("ranges", "", "Comma-separated Open-Closed ranges of the form L:R")

	// nInfo holds nodeInfo data for the tree.
	nInfo = make(map[string]nodeInfo)
)

// nodeInfo represents the style to be applied to a tree node.
type nodeInfo struct {
	incProof     bool
	incPath      bool
	target       bool
	perfectRoot  bool
	ephemeral    bool
	leaf         bool
	rangeIndices []int
}

// String returns a string containing Forest attributes suitable for
// rendering the node, given its type.
func (n nodeInfo) String() string {
	attr := make([]string, 0, 4)

	// Figure out which colour to fill with:
	fill := "white"
	if n.perfectRoot {
		attr = append(attr, "line width=4pt")
	}
	if n.incProof {
		fill = "inclusion"
		if n.ephemeral {
			fill = "inclusion_ephemeral"
			attr = append(attr, "draw, dotted")
		}
	}
	if n.target {
		fill = "target"
	}
	if n.incPath {
		fill = "target_path"
	}

	if len(n.rangeIndices) == 1 {
		// For nodes in a single range just change the fill
		fill = fmt.Sprintf("range%d!50", n.rangeIndices[0])
	} else {
		// Otherwise, we need to be a bit cleverer, and use the shading feature
		for i, ri := range n.rangeIndices {
			pos := []string{"left", "right", "middle"}[i]
			attr = append(attr, fmt.Sprintf("%s color=range%d!50", pos, ri))
		}
	}
	attr = append(attr, "fill="+fill)

	if !n.ephemeral {
		attr = append(attr, "draw")
	}
	if !n.leaf {
		attr = append(attr, "circle, minimum size=3em")
	} else {
		attr = append(attr, "minimum size=1.5em")
	}
	return strings.Join(attr, ", ")
}

// modifyNodeInfo applies f to the nodeInfo associated with node k.
func modifyNodeInfo(k string, f func(*nodeInfo)) {
	n, ok := nInfo[k]
	if !ok {
		n = nodeInfo{}
	}
	f(&n)
	nInfo[k] = n
}

// perfectMega renders a large perfect subtree as a single entity.
func perfectMega(prefix string, height, leafIndex int64) {
	stLeaves := int64(1) << uint(height)
	stWidth := float32(stLeaves) / float32(*treeSize)
	fmt.Printf("%s [%d\\dots%d, edge label={node[midway, above]{%d}}, perfect, tier=leaf, minimum width=%f\\linewidth ]\n", prefix, leafIndex, leafIndex+stLeaves, stLeaves, stWidth)

	// Create some hidden nodes to preseve the tier spacings:
	fmt.Printf("%s", prefix)
	for i := height - 2; i > 0; i-- {
		fmt.Printf(" [, no edge, tier=%d ", i)
		defer fmt.Printf(" ] ")
	}
}

// perfect renders a perfect subtree.
func perfect(prefix string, height, index int64) {
	perfectInner(prefix, height, index, true)
}

// drawLeaf emits TeX code to render a leaf.
func drawLeaf(prefix string, index int64) {
	a := nInfo[nodeKey(0, index)]
	fmt.Printf("%s [%d, %s, tier=leaf]\n", prefix, index, a.String())
}

// openInnerNode renders TeX code to open an internal node.
// The caller may emit any number of child nodes before calling the returned
// func to close the node.
// Returns a func to be called to close the node.
//
func openInnerNode(prefix string, height, index int64) func() {
	attr := nInfo[nodeKey(height, index)].String()
	fmt.Printf("%s [%d.%d, %s, tier=%d\n", prefix, height, index, attr, height-1)
	return func() { fmt.Printf("%s ]\n", prefix) }
}

// perfectInner renders the nodes of a perfect internal subtree.
func perfectInner(prefix string, height, index int64, top bool) {
	nk := nodeKey(height, index)
	modifyNodeInfo(nk, func(n *nodeInfo) {
		n.leaf = height == 0
		n.perfectRoot = top
	})

	if height == 0 {
		drawLeaf(prefix, index)
		return
	}
	c := openInnerNode(prefix, height, index)
	childIndex := index << 1
	if height > *megaMode {
		perfectMega(prefix, height, index<<uint(height))
	} else {
		perfectInner(prefix+" ", height-1, childIndex, false)
		perfectInner(prefix+" ", height-1, childIndex+1, false)
	}
	c()
}

// renderTree renders a tree node and recurses if necessary.
func renderTree(prefix string, treeSize, index int64) {
	if treeSize <= 0 {
		return
	}

	// Look at the bit of the treeSize corresponding to the current level:
	height := int64(bits.Len64(uint64(treeSize)) - 1)
	b := int64(1) << uint(height)
	rest := treeSize - b
	// left child is a perfect subtree

	// if there's a right-hand child, then we'll emit this node to be the
	// parent. (Otherwise we'll just keep quiet, and recurse down - this is how
	// we arrange for leaves to always be on the bottom level.)
	if rest > 0 {
		ch := height + 1
		ci := index >> uint(ch)
		modifyNodeInfo(nodeKey(ch, ci), func(n *nodeInfo) { n.ephemeral = true })
		c := openInnerNode(prefix, ch, ci)
		defer c()
	}
	perfect(prefix+" ", height, index>>uint(height))
	index += b
	renderTree(prefix+" ", rest, index)
}

// nodeKey returns a stable node identifier for the passed in node coordinate.
func nodeKey(height, index int64) string {
	return fmt.Sprintf("%d.%d", height, index)
}

// toNodeKey converts a storage.NodeID to the corresponding stable node
// identifier used by this tool.
func toNodeKey(n storage.NodeID) string {
	d := int64(maxLen - n.PrefixLenBits)
	i := n.BigInt().Int64() >> uint(d)
	return nodeKey(d, i)
}

// decompose splits out a range into component subtrees.
// TODO(al): Remove this and depend on actual range code.
func decomposeRange(begin, end uint64) (uint64, uint64) {
	// Special case, as the code below works only if begin != 0, or end < 2^63.
	if begin == 0 {
		return 0, end
	}
	xbegin := begin - 1
	// Find where paths to leaves #begin-1 and #end diverge, and mask the upper
	// bits away, as only the nodes strictly below this point are in the range.
	d := bits.Len64(xbegin^end) - 1
	mask := uint64(1)<<uint(d) - 1
	// The left part of the compact range consists of all nodes strictly below
	// and to the right from the path to leaf #begin-1, corresponding to zero
	// bits in the masked part of begin-1. Likewise, the right part consists of
	// nodes below and to the left from the path to leaf #end, corresponding to
	// ones in the masked part of end.
	return ^xbegin & mask, end & mask
}

// parseRanges parses and validates a string of comma-separates open-closed
// ranges of the form L:R.
// Returns the parsed ranges, or an error if there's a problem.
func parseRanges(ranges string, treeSize int64) ([][2]int64, error) {
	rangePairs := strings.Split(ranges, ",")
	numRanges := len(rangePairs)
	if num, max := numRanges, maxRanges; num > max {
		return nil, fmt.Errorf("too many ranges %d, must be %d or fewer", num, max)
	}
	ret := make([][2]int64, 0, numRanges)
	for _, rng := range rangePairs {
		lr := strings.Split(rng, ":")
		if len(lr) != 2 {
			return nil, fmt.Errorf("specified range %q is invalid", rng)
		}
		var l, r int64
		if _, err := fmt.Sscanf(rng, "%d:%d", &l, &r); err != nil {
			return nil, fmt.Errorf("range (%q) is malformed: %s", rng, err)
		}
		if r > treeSize {
			return nil, fmt.Errorf("range %q extends past end of tree (%d)", lr, treeSize)
		}
		if l < 0 {
			return nil, fmt.Errorf("range %q has -ve element", rng)
		}
		if l > r {
			return nil, fmt.Errorf("range elements in %q are out of order", rng)
		}
		ret = append(ret, [2]int64{l, r})
	}
	return ret, nil
}

// modifyRangeNodeInfo sets style info for nodes affected by ranges.
// This includes leaves and perfect subtree roots.
// TODO(al): Figure out what, if anything, to do to make this show ranges
// which are inside the perfect meganodes.
func modifyRangeNodeInfo() error {
	rng, err := parseRanges(*ranges, *treeSize)
	if err != nil {
		return err
	}
	for ri, lr := range rng {
		rStyle := ri + 1
		l, r := lr[0], lr[1]
		// Set leaves:
		for i := l; i < r; i++ {
			modifyNodeInfo(nodeKey(0, i), func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, rStyle) })
		}

		// Now perfect roots which comprise the range:
		maskL, maskR := decomposeRange(uint64(l), uint64(r))
		p := l
		// Do left perfect subtree roots:
		nBitsL := uint64(bits.Len64(maskL))
		for i, bit := uint64(0), uint64(1); i < nBitsL; i, bit = i+1, bit<<1 {
			if maskL&bit != 0 {
				if i > 0 {
					modifyNodeInfo(nodeKey(int64(i), p>>i), func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, rStyle) })
				}
				p += int64(bit)
			}
		}
		// Do right perfect subtree roots:
		nBitsR := uint(bits.Len64(maskR))
		for i, bit := nBitsR, uint64(1<<nBitsR); i > 1; i, bit = i-1, bit>>1 {
			if maskR&bit != 0 {
				modifyNodeInfo(nodeKey(int64(i), p>>i), func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, rStyle) })
				p += int64(bit)
			}
		}
	}
	return nil
}

// Whee - here we go!
func main() {
	// TODO(al): check flag validity.
	flag.Parse()
	height := int64(bits.Len(uint(*treeSize-1)) + 1)

	if *inclusion > 0 {
		modifyNodeInfo(nodeKey(0, *inclusion), func(n *nodeInfo) { n.target = true })
		nf, err := merkle.CalcInclusionProofNodeAddresses(*treeSize, *inclusion, *treeSize, maxLen)
		if err != nil {
			log.Fatalf("Failed to calculate inclusion proof addresses: %s", err)
		}
		for _, n := range nf {
			modifyNodeInfo(toNodeKey(n.NodeID), func(n *nodeInfo) { n.incProof = true })
		}
		for h, i := int64(0), *inclusion; h < height; h, i = h+1, i>>1 {
			modifyNodeInfo(nodeKey(h, i), func(n *nodeInfo) { n.incPath = true })
		}
	}

	if len(*ranges) > 0 {
		if err := modifyRangeNodeInfo(); err != nil {
			log.Fatalf("Failed to modify range node styles: %s", err)
		}
	}

	// TODO(al): structify this into a util, and add ability to output to an
	// arbitrary stream.
	fmt.Print(preamble)
	renderTree("", *treeSize, 0)
	fmt.Println(postfix)
}

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
	"github.com/google/trillian/merkle/compact"
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
\definecolor{range0}{rgb}{0.3,0.9,0.3}
\definecolor{range1}{rgb}{0.3,0.3,0.9}
\definecolor{range2}{rgb}{0.9,0.3,0.9}

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
)

var (
	treeSize   = flag.Uint64("tree_size", 23, "Size of tree to produce")
	leafData   = flag.String("leaf_data", "", "Comma separated list of leaf data text (setting this overrides --tree_size")
	nodeFormat = flag.String("node_format", "address", "Format for internal node text, one of: address, hash")
	inclusion  = flag.Int64("inclusion", -1, "Leaf index to show inclusion proof")
	megaMode   = flag.Uint("megamode_threshold", 4, "Treat perfect trees larger than this many layers as a single entity")
	ranges     = flag.String("ranges", "", "Comma-separated Open-Closed ranges of the form L:R")

	attrPerfectRoot   = flag.String("attr_perfect_root", "line width=4pt", "Latex treatment for perfect root nodes")
	attrEphemeralNode = flag.String("attr_ephemeral_node", "draw, dotted", "Latex treatment for ephemeral nodes")

	// nInfo holds nodeInfo data for the tree.
	nInfo = make(map[compact.NodeID]nodeInfo)
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

type nodeTextFunc func(id compact.NodeID) string

// String returns a string containing Forest attributes suitable for
// rendering the node, given its type.
func (n nodeInfo) String() string {
	attr := make([]string, 0, 4)

	// Figure out which colour to fill with:
	fill := "white"
	if n.perfectRoot {
		attr = append(attr, *attrPerfectRoot)
	}
	if n.incProof {
		fill = "inclusion"
		if n.ephemeral {
			fill = "inclusion_ephemeral"
			attr = append(attr, *attrEphemeralNode)
		}
	}
	if n.target {
		fill = "target"
	}
	if n.incPath {
		fill = "target_path"
	}

	if len(n.rangeIndices) == 1 {
		// For nodes in a single range just change the fill.
		fill = fmt.Sprintf("range%d!50", n.rangeIndices[0])
	} else {
		// Otherwise, we need to be a bit cleverer, and use the shading feature.
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
		attr = append(attr, "circle, minimum size=3em, align=center")
	} else {
		attr = append(attr, "minimum size=1.5em, align=center, base=bottom")
	}
	return strings.Join(attr, ", ")
}

// modifyNodeInfo applies f to the nodeInfo associated with node id.
func modifyNodeInfo(id compact.NodeID, f func(*nodeInfo)) {
	n := nInfo[id] // Note: Returns an empty nodeInfo if id is not found.
	f(&n)
	nInfo[id] = n
}

// perfectMega renders a large perfect subtree as a single entity.
func perfectMega(prefix string, height uint, leafIndex uint64) {
	stLeaves := uint64(1) << height
	stWidth := float32(stLeaves) / float32(*treeSize)
	fmt.Printf("%s [%d\\dots%d, edge label={node[midway, above]{%d}}, perfect, tier=leaf, minimum width=%f\\linewidth ]\n", prefix, leafIndex, leafIndex+stLeaves, stLeaves, stWidth)

	// Create some hidden nodes to preseve the tier spacings:
	fmt.Printf("%s", prefix)
	for i := int(height - 2); i > 0; i-- {
		fmt.Printf(" [, no edge, tier=%d ", i)
		defer fmt.Printf(" ] ")
	}
}

// perfect renders a perfect subtree.
func perfect(prefix string, height uint, index uint64, nodeText, dataText nodeTextFunc) {
	perfectInner(prefix, height, index, true, nodeText, dataText)
}

// drawLeaf emits TeX code to render a leaf.
func drawLeaf(prefix string, index uint64, leafText, dataText string) {
	a := nInfo[compact.NewNodeID(0, index)]

	// First render the leaf node of the Merkle tree
	fmt.Printf("%s [%s, %s, align=center, tier=leaf\n", prefix, leafText, a.String())
	// and then a child-node representing the leaf data itself:
	a.leaf = true
	fmt.Printf("  %s [%s, %s, align=center, tier=leafdata]\n]\n", prefix, dataText, a.String())
}

// openInnerNode renders TeX code to open an internal node.
// The caller may emit any number of child nodes before calling the returned
// func to close the node.
// Returns a func to be called to close the node.
func openInnerNode(prefix string, id compact.NodeID, text string) func() {
	attr := nInfo[id].String()
	fmt.Printf("%s [%s, %s, tier=%d\n", prefix, text, attr, id.Level)
	return func() { fmt.Printf("%s ]\n", prefix) }
}

// perfectInner renders the nodes of a perfect internal subtree.
func perfectInner(prefix string, level uint, index uint64, top bool, nodeText nodeTextFunc, dataText nodeTextFunc) {
	id := compact.NewNodeID(level, index)
	modifyNodeInfo(id, func(n *nodeInfo) {
		n.perfectRoot = top
	})

	if level == 0 {
		leafNodeText := nodeText(id)
		dataNodeText := dataText(id)
		drawLeaf(prefix, index, leafNodeText, dataNodeText)
		return
	}
	text := nodeText(id)
	c := openInnerNode(prefix, id, text)
	childIndex := index << 1
	if level > *megaMode {
		perfectMega(prefix, level, index<<level)
	} else {
		perfectInner(prefix+" ", level-1, childIndex, false, nodeText, dataText)
		perfectInner(prefix+" ", level-1, childIndex+1, false, nodeText, dataText)
	}
	c()
}

// renderTree renders a tree node and recurses if necessary.
func renderTree(prefix string, treeSize, index uint64, nodeText, dataText nodeTextFunc) {
	if treeSize == 0 {
		return
	}

	// Look at the bit of the treeSize corresponding to the current level:
	height := uint(bits.Len64(treeSize) - 1)
	b := uint64(1) << height
	rest := treeSize - b
	// left child is a perfect subtree.

	// if there's a right-hand child, then we'll emit this node to be the
	// parent. (Otherwise we'll just keep quiet, and recurse down - this is how
	// we arrange for leaves to always be on the bottom level.)
	if rest > 0 {
		childHeight := height + 1
		id := compact.NewNodeID(childHeight, index>>childHeight)
		modifyNodeInfo(id, func(n *nodeInfo) { n.ephemeral = true })
		c := openInnerNode(prefix, id, nodeText(id))
		defer c()
	}
	perfect(prefix+" ", height, index>>height, nodeText, dataText)
	index += b
	renderTree(prefix+" ", rest, index, nodeText, dataText)
}

// parseRanges parses and validates a string of comma-separates open-closed
// ranges of the form L:R.
// Returns the parsed ranges, or an error if there's a problem.
func parseRanges(ranges string, treeSize uint64) ([][2]uint64, error) {
	rangePairs := strings.Split(ranges, ",")
	numRanges := len(rangePairs)
	if num, max := numRanges, maxRanges; num > max {
		return nil, fmt.Errorf("too many ranges %d, must be %d or fewer", num, max)
	}
	ret := make([][2]uint64, 0, numRanges)
	for _, rng := range rangePairs {
		lr := strings.Split(rng, ":")
		if len(lr) != 2 {
			return nil, fmt.Errorf("specified range %q is invalid", rng)
		}
		var l, r uint64
		if _, err := fmt.Sscanf(rng, "%d:%d", &l, &r); err != nil {
			return nil, fmt.Errorf("range %q is malformed: %s", rng, err)
		}
		switch {
		case r > treeSize:
			return nil, fmt.Errorf("range %q extends past end of tree (%d)", lr, treeSize)
		case l > r:
			return nil, fmt.Errorf("range elements in %q are out of order", rng)
		}
		ret = append(ret, [2]uint64{l, r})
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
		l, r := lr[0], lr[1]
		// Set leaves:
		for i := l; i < r; i++ {
			id := compact.NewNodeID(0, i)
			modifyNodeInfo(id, func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, ri) })
		}

		// Now perfect roots which comprise the range:
		maskL, maskR := compact.Decompose(l, r)
		p := l
		// Do left perfect subtree roots:
		nBitsL := uint(bits.Len64(maskL))
		for i, bit := uint(0), uint64(1); i < nBitsL; i, bit = i+1, bit<<1 {
			if maskL&bit != 0 {
				if i > 0 {
					id := compact.NewNodeID(i, p>>i)
					modifyNodeInfo(id, func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, ri) })
				}
				p += bit
			}
		}
		// Do right perfect subtree roots:
		nBitsR := uint(bits.Len64(maskR))
		for i, bit := nBitsR, uint64(1<<nBitsR); i > 1; i, bit = i-1, bit>>1 {
			if maskR&bit != 0 {
				id := compact.NewNodeID(i, p>>i)
				modifyNodeInfo(id, func(n *nodeInfo) { n.rangeIndices = append(n.rangeIndices, ri) })
				p += bit
			}
		}
	}
	return nil
}

var dataFormat = func(id compact.NodeID) string {
	return fmt.Sprintf("{$leaf_{%d}$}", id.Index)
}

var nodeFormats = map[string]nodeTextFunc{
	"address": func(id compact.NodeID) string {
		return fmt.Sprintf("%d.%d", id.Level, id.Index)
	},
	"hash": func(id compact.NodeID) string {
		if id.Level >= 1 {
			childLevel := id.Level - 1
			leftChild := id.Index * 2
			return fmt.Sprintf("{$H_{%d.%d} =$ \\\\ $H(H_{%d.%d} || H_{%d.%d})$}", id.Level, id.Index, childLevel, leftChild, childLevel, leftChild+1)
		}
		return fmt.Sprintf("{$H_{%d.%d} =$ \\\\ $H(leaf_{%[2]d})$}", id.Level, id.Index)
	},
}

// Whee - here we go!
func main() {
	// TODO(al): check flag validity.
	flag.Parse()
	height := uint(bits.Len64(*treeSize-1)) + 1

	innerNodeText := nodeFormats[*nodeFormat]
	if innerNodeText == nil {
		log.Fatalf("unknown --node_format %s", *nodeFormat)
	}

	var nodeText nodeTextFunc = func(id compact.NodeID) string {
		return innerNodeText(id)
	}

	if len(*leafData) > 0 {
		leaves := strings.Split(*leafData, ",")
		*treeSize = uint64(len(leaves))
		log.Printf("Overriding treeSize to %d since --leaf_data was set", *treeSize)
		nodeText = func(id compact.NodeID) string {
			if id.Level == 0 {
				return leaves[id.Index]
			}
			return innerNodeText(id)
		}
	}

	if *inclusion > 0 {
		leafID := compact.NewNodeID(0, uint64(*inclusion))
		modifyNodeInfo(leafID, func(n *nodeInfo) { n.target = true })
		nf, err := merkle.CalcInclusionProofNodeAddresses(int64(*treeSize), *inclusion, int64(*treeSize))
		if err != nil {
			log.Fatalf("Failed to calculate inclusion proof addresses: %s", err)
		}
		for _, n := range nf {
			modifyNodeInfo(n.ID, func(n *nodeInfo) { n.incProof = true })
		}
		for h, i := uint(0), leafID.Index; h < height; h, i = h+1, i>>1 {
			id := compact.NewNodeID(h, i)
			modifyNodeInfo(id, func(n *nodeInfo) { n.incPath = true })
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
	renderTree("", *treeSize, 0, nodeText, dataFormat)
	fmt.Print(postfix)
}

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

\begin{forest}
`

	postfix = `\end{forest}
\end{document}
`
	// maxLen is a suitably large maximum nodeID length for storage.NodeID.
	maxLen = 64
)

var (
	treeSize  = flag.Int64("tree_size", 23, "Size of tree to produce")
	inclusion = flag.Int64("inclusion", -1, "Leaf index to show inclusion proof")

	// nInfo holds nodeInfo data for the tree.
	nInfo = make(map[string]nodeInfo)
)

// nodeInfo represents the style to be applied to a tree node.
type nodeInfo struct {
	incProof    bool
	incPath     bool
	target      bool
	perfectRoot bool
	ephemeral   bool
	leaf        bool
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
// func to clode the node.
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
	perfectInner(prefix+" ", height-1, childIndex, false)
	perfectInner(prefix+" ", height-1, childIndex+1, false)
	c()
}

// renderTree renders a tree node and recurses if necessary.
func renderTree(prefix string, treeSize, index int64) {
	height := int64(bits.Len64(uint64(treeSize)) - 1)
	if height < 0 || treeSize <= 0 {
		return
	}

	// Look at the bit of the treeSize corresponding to the current level:
	b := (1 << uint(height)) & treeSize
	rest := treeSize - b
	if b > 0 {
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
	}
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

	// TODO(al): structify this into a util, and add ability to output to an
	// arbitrary stream.
	fmt.Print(preamble)
	renderTree("", *treeSize, 0)
	fmt.Println(postfix)
}

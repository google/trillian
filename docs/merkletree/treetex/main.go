// treetek is a command to produce LaTeX documents representing merkle trees.
// Uses the Forest package.
//
// Usage: go run main.go | xelatex
// This should generate a PDF file called treetek.pdf containing a drawing of
// the tree.
//
package main

import (
	"flag"
	"fmt"
	"math/bits"
	"strings"

	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
)

const (
	preamble = `
% Hash-tree
% Author: treetek
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

// setNodeInfo applies f to the nodeInfo associated with node k.
func setNodeInfo(k string, f func(*nodeInfo)) {
	n, ok := nInfo[k]
	if !ok {
		n = nodeInfo{}
	}
	f(&n)
	nInfo[k] = n
}

// perfect renders a perfect subtree.
func perfect(prefix string, height, tier, index int64) {
	perfectInner(prefix, height, tier, index, true)
}

// drawLeaf emits TeX code to render a leaf.
func drawLeaf(prefix string, index int64) {
	a := nInfo[nodeKey(0, index)]
	fmt.Printf("%s [%d, %s, tier=leaf]\n", prefix, index, a.String())
}

// openInnerNode renders tex code to open an internal node.
// The caller may emit any number of child nodes before calling the returned
// func to clode the node.
// returns a func to be called to close the node.
//
func openInnerNode(prefix string, height, index, tier int64) func() {
	attr := nInfo[nodeKey(height, index)].String()
	fmt.Printf("%s [%d.%d, %s, tier=%d\n", prefix, height, index, attr, tier)
	return func() { fmt.Printf("%s ]\n", prefix) }
}

// perfectInner renders the nodes of a perfect internal subtree.
func perfectInner(prefix string, height, tier, index int64, top bool) {
	nk := nodeKey(height, index)
	setNodeInfo(nk, func(n *nodeInfo) { n.leaf = height == 0 })
	setNodeInfo(nodeKey(height, index), func(n *nodeInfo) { n.perfectRoot = top })

	if height == 0 {
		drawLeaf(prefix, index)
		return
	}
	c := openInnerNode(prefix, height, index, tier)
	childIndex := index << 1
	perfectInner(prefix+" ", height-1, tier-1, childIndex, false)
	perfectInner(prefix+" ", height-1, tier-1, childIndex+1, false)
	c()
}

// node renders a tree node.
func node(prefix string, treeSize, height, tier, index int64) {
	if height < 0 {
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
			setNodeInfo(nodeKey(ch, ci), func(n *nodeInfo) { n.ephemeral = true })
			c := openInnerNode(prefix, ch, ci, tier)
			defer c()
		}
		perfect(prefix+" ", height, tier-1, index>>uint(tier))
		index += b
	}
	if rest == 0 {
		return
	}
	node(prefix+" ", rest, height-1, tier-1, index)
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
	flag.Parse()
	height := int64(bits.Len(uint(*treeSize)))

	if *inclusion > 0 {
		setNodeInfo(nodeKey(0, *inclusion), func(n *nodeInfo) { n.target = true })
		nf, err := merkle.CalcInclusionProofNodeAddresses(*treeSize, *inclusion, *treeSize, maxLen)
		if err != nil {
			panic(err)
		}
		for _, n := range nf {
			setNodeInfo(toNodeKey(n.NodeID), func(n *nodeInfo) { n.incProof = true })
		}
		h := int64(0)
		i := *inclusion
		for h < height {
			setNodeInfo(nodeKey(h, i), func(n *nodeInfo) { n.incPath = true })
			h++
			i >>= 1
		}
	}

	fmt.Print(preamble)
	node("", *treeSize, height, height, 0)
	fmt.Println(postfix)
}

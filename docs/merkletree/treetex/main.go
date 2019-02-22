/* treetek is a command to produce LaTeX documents representing merkle trees.
 * Uses the Forest package.
 *
 * Usage: go run main.go | xelatex
 * This should generate a PDF file called treetek.pdf containing a drawing of
 * the tree.
 */
package main

import (
	"flag"
	"fmt"
	"math/bits"
)

const (
	preamble = `
% Hash-tree
% Author: treetek
\documentclass[convert]{standalone}
\usepackage[dvipsnames]{xcolor}
\usepackage{forest}


\begin{document}

\begin{forest}
`

	postfix = `\end{forest}
\end{document}
`
)

var (
	treeSize = flag.Int("tree_size", 23, "Size of tree to produce")
)

/* perfect renders a perfect subtree.
 */
func perfect(prefix string, height, tier, index int) {
	perfectInner(prefix, height, tier, index, true)
}

/* drawLeaf emits TeX code to render a leaf.
 */
func drawLeaf(prefix string, index, tier int) {
	fmt.Printf("%s [%d, draw, tier=%d]\n", prefix, index, tier)
}

/* openInnerNode renders tex code to open an internal node.
 * The caller may emit any number of child nodes before calling the returned
 * func to clode the node.
 * returns a func to be called to close the node.
 */
func openInnerNode(prefix string, height, index, tier int, attr string) func() {
	fmt.Printf("%s [%d.%d, %s, tier=%d\n", prefix, height, index, attr, tier)
	return func() { fmt.Printf("%s ]\n", prefix) }
}

/* perfectInner renders the nodes of a perfect internal subtree.
 */
func perfectInner(prefix string, height, tier, index int, top bool) {
	if height == 0 {
		drawLeaf(prefix, index, tier)
		return
	}
	attr := "draw, circle, fill="
	if top {
		attr += "Goldenrod"
	} else {
		attr += "white"
	}
	c := openInnerNode(prefix, height, index, tier, attr)
	childIndex := index << 1
	perfectInner(prefix+" ", height-1, tier-1, childIndex, false)
	perfectInner(prefix+" ", height-1, tier-1, childIndex+1, false)
	c()
}

/* node renders a tree node.
 */
func node(prefix string, treeSize, height, tier, index int) {
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
			c := openInnerNode(prefix, height+1, index>>uint(height+1), tier, "")
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

func main() {
	flag.Parse()
	height := bits.Len(uint(*treeSize))

	fmt.Print(preamble)
	node("", *treeSize, height, height, 0)
	fmt.Println(postfix)
}

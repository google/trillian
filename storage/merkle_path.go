package storage

import "fmt"

// CalcInclusionProofNodeAddresses returns the tree node IDs needed to
// build an inclusion proof for a specified leaf and tree size. The maxBitLen parameter
// is copied into all the returned nodeIDs.
func CalcInclusionProofNodeAddresses(treeSize, index int64, maxBitLen int) ([]NodeID, error) {
	if index >= treeSize || index < 0 || treeSize < 1 || maxBitLen < 0 {
		return []NodeID{}, fmt.Errorf("invalid params ts: %d index: %d, bitlen:%d", treeSize, index, maxBitLen)
	}

	var proof []NodeID

	sizeLessOne := treeSize - 1

	if bitLen(treeSize) == 0 || index > sizeLessOne {
		return proof, nil
	}

	node := index
	depth := 0
	lastNodeAtLevel := sizeLessOne

	for depth < bitLen(sizeLessOne) {
		sibling := node ^ 1
		if sibling < lastNodeAtLevel {
			// Tree must be completely filled in up to this node index
			proof = append(proof, NewNodeIDForTreeCoords(int64(depth), sibling, maxBitLen))
		} else if sibling == lastNodeAtLevel {
			// The tree may skip levels because it's not completely filled in. These nodes
			// don't exist
			drop := depth - subtreeDepth(treeSize, depth - 1)
			sibling = sibling << uint(drop)
			proof = append(proof, NewNodeIDForTreeCoords(int64(depth - drop), sibling, maxBitLen))
		}

		node = node >> 1
		lastNodeAtLevel = lastNodeAtLevel >> 1
		depth++
	}

	return proof, nil
}

// bitLen returns the number of bits needed to represent the supplied integer
func bitLen(x int64) int {
	l := 0

	for x > 0 {
		x = x >> 1
		l++
	}

	return l
}

// subtreeDepth calculates the depth of a subtree, used at the right of the tree which
// may not be completely populated
func subtreeDepth(size int64, bits int) int {
	for b := bitLen(size) - 1; b > bits; b-- {
		size = size &^ (1 << uint(b))
	}

	// determine tree height for the remaining bits.
	p2 := bitLen(size) - 1
	size = size &^ (1 << uint(p2))
	if bitLen(size) > 0 {
		p2++
	}

	return p2
}

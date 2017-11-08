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

package merkle

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/google/trillian/merkle/hashers"
)

// RootMismatchError occurs when an inclusion proof fails.
type RootMismatchError struct {
	ExpectedRoot   []byte
	CalculatedRoot []byte
}

func (e RootMismatchError) Error() string {
	return fmt.Sprintf("calculated root:\n%v\n does not match expected root:\n%v", e.CalculatedRoot, e.ExpectedRoot)
}

// LogVerifier verifies inclusion and consistency proofs for append only logs.
type LogVerifier struct {
	hasher hashers.LogHasher
}

// NewLogVerifier returns a new LogVerifier for a tree.
func NewLogVerifier(hasher hashers.LogHasher) LogVerifier {
	return LogVerifier{
		hasher: hasher,
	}
}

// VerifyInclusionProof verifies the correctness of the proof given the passed in information about the tree and leaf.
func (v LogVerifier) VerifyInclusionProof(leafIndex, treeSize int64, proof [][]byte, root []byte, leafHash []byte) error {
	calcRoot, err := v.RootFromInclusionProof(leafIndex, treeSize, proof, leafHash)
	if err != nil {
		return err
	}
	if !bytes.Equal(calcRoot, root) {
		return RootMismatchError{
			CalculatedRoot: calcRoot,
			ExpectedRoot:   root,
		}
	}
	return nil
}

// RootFromInclusionProof calculates the expected tree root given the proof and leaf.
// leafIndex starts at 0.  treeSize is the number of nodes in the tree.
// proof is an array of neighbor nodes from the bottom to the root.
func (v LogVerifier) RootFromInclusionProof(leafIndex, treeSize int64, proof [][]byte, leafHash []byte) ([]byte, error) {
	if leafIndex < 0 {
		return nil, errors.New("invalid leafIndex < 0")
	}
	if treeSize < 0 {
		return nil, errors.New("invalid treeSize < 0")
	}
	lastIndex := treeSize - 1 // Rightmost node in tree.
	if leafIndex > lastIndex {
		return nil, fmt.Errorf("leafIndex is not in a tree of size %d, want %d<%d", treeSize, leafIndex, treeSize)
	}

	cntIndex := leafIndex
	cntHash := leafHash
	proofIndex := 0

	// Tree is numbered as follows, where nodes at each level are counted from left to right.
	//       0
	//     0   1
	//   0  1 2  3

	// Hash sibling nodes into the current hash starting at the leaf and continuing to the root.
	// Use the highest order 1 bit in the rightmost node as the stopping condition.
	for lastIndex > 0 {
		if proofIndex >= len(proof) {
			return nil, fmt.Errorf("insuficient number of proof components (%d) for treeSize %d", len(proof), treeSize)
		}
		if isRightChild(cntIndex) {
			cntHash = v.hasher.HashChildren(proof[proofIndex], cntHash)
			proofIndex++
		} else if cntIndex < lastIndex {
			cntHash = v.hasher.HashChildren(cntHash, proof[proofIndex])
			proofIndex++
		} // else the sibling does not exist.
		cntIndex = parent(cntIndex)
		lastIndex = parent(lastIndex)
	}
	if proofIndex != len(proof) {
		return nil, fmt.Errorf("invalid proof, expected %d components, but have %d", proofIndex, len(proof))
	}
	return cntHash, nil
}

// VerifyConsistencyProof checks that the passed in consistency proof is valid between the passed in tree snapshots.
// Snapshots are the respective treeSizes. shapshot2 >= snapshot1 >= 0.
func (v LogVerifier) VerifyConsistencyProof(snapshot1, snapshot2 int64, root1, root2 []byte, proof [][]byte) error {
	if snapshot1 < 0 {
		return fmt.Errorf("snapshot1 (%d) < 0 ", snapshot1)
	}
	if snapshot2 < snapshot1 {
		return fmt.Errorf("snapshot2 (%d) < snapshot1 (%d)", snapshot1, snapshot2)
	}
	if snapshot1 == snapshot2 {
		if !bytes.Equal(root1, root2) {
			return RootMismatchError{
				CalculatedRoot: root1,
				ExpectedRoot:   root2,
			}
		}
		if len(proof) > 0 {
			return errors.New("root1 and root2 match, but proof is non-empty")
		}
		// proof ok.
		return nil
	}
	if snapshot1 == 0 {
		// Any snapshot greater than 0 is consistent with snapshot 0.
		if len(proof) > 0 {
			return fmt.Errorf("expected empty proof, but provided proof has %d components", len(proof))
		}
		return nil
	}
	if len(proof) == 0 {
		return errors.New("empty proof")
	}

	node := snapshot1 - 1
	lastNode := snapshot2 - 1
	proofIndex := 0

	for isRightChild(node) {
		node = parent(node)
		lastNode = parent(lastNode)
	}

	var node1Hash []byte
	var node2Hash []byte

	if node > 0 {
		node1Hash = proof[proofIndex]
		node2Hash = proof[proofIndex]
		proofIndex++
	} else {
		// The tree at snapshot1 was balanced, nothing to verify for root1.
		node1Hash = root1
		node2Hash = root1
	}

	// Use the highest order 1 bit in the rightmost node of snapshot1 as the stopping condition.
	for node > 0 {
		if proofIndex >= len(proof) {
			return errors.New("insufficient number of proof components")
		}

		if isRightChild(node) {
			node1Hash = v.hasher.HashChildren(proof[proofIndex], node1Hash)
			node2Hash = v.hasher.HashChildren(proof[proofIndex], node2Hash)
			proofIndex++
		} else if node < lastNode {
			// Test whether a sibling node to the right exists at this level.
			node2Hash = v.hasher.HashChildren(node2Hash, proof[proofIndex])
			proofIndex++
		} // else the sibling does not exist.

		node = parent(node)
		lastNode = parent(lastNode)
	}

	// Verify the first root.
	if !bytes.Equal(node1Hash, root1) {
		return RootMismatchError{
			CalculatedRoot: node1Hash,
			ExpectedRoot:   root1,
		}
	}

	// Use the highest order 1 bit in the rightmost node of snapshot2 as the stopping condition.
	for lastNode > 0 {
		if proofIndex >= len(proof) {
			return errors.New("can't verify newer root; insufficient number of proof components")
		}

		node2Hash = v.hasher.HashChildren(node2Hash, proof[proofIndex])
		proofIndex++
		lastNode = parent(lastNode)
	}

	// Verify the second root.
	if !bytes.Equal(node2Hash, root2) {
		return RootMismatchError{
			CalculatedRoot: node2Hash,
			ExpectedRoot:   root2,
		}
	}
	if proofIndex != len(proof) {
		return errors.New("proof has too many components")
	}

	return nil // Proof OK.
}

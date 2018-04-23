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
	"math/bits"

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

// VerifyInclusionProof verifies the correctness of the proof given the passed
// in information about the tree and leaf.
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

func validateLeafIndex(leafIndex, treeSize int64) error {
	switch {
	case leafIndex < 0:
		return fmt.Errorf("leafIndex %d < 0", leafIndex)
	case treeSize < 0:
		return fmt.Errorf("treeSize %d < 0", treeSize)
	case leafIndex >= treeSize:
		return fmt.Errorf("leafIndex is beyond treeSize: %d >= %d", leafIndex, treeSize)
	}
	return nil
}

// RootFromInclusionProof calculates the expected tree root given the proof and leaf.
// leafIndex starts at 0.  treeSize is the number of nodes in the tree.
// proof is an array of neighbor nodes from the bottom to the root.
func (v LogVerifier) RootFromInclusionProof(leafIndex, treeSize int64, proof [][]byte, leafHash []byte) ([]byte, error) {
	if err := validateLeafIndex(leafIndex, treeSize); err != nil {
		return nil, err
	}

	inner, border := decompInclProof(leafIndex, treeSize)
	if got, want := len(proof), inner+border; got != want {
		return nil, fmt.Errorf("invalid proof: want %d components, got %d", want, got)
	}

	res := leafHash
	for i, h := range proof {
		if i < inner && (leafIndex>>uint(i))&1 == 0 {
			res = v.hasher.HashChildren(res, h)
		} else {
			res = v.hasher.HashChildren(h, res)
		}
	}
	return res, nil
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

// VerifiedPrefixHashFromInclusionProof calculates a root hash over leaves
// [0..subSize), based on the inclusion |proof| and |leafHash| for a leaf at
// the |subSize| index in a tree of the specified |size| with the passed in
// |root| hash.
// Returns an error if the |proof| verification fails. The resulting smaller
// tree's root hash is trusted iff the bigger tree's |root| hash is trusted.
func (v LogVerifier) VerifiedPrefixHashFromInclusionProof(
	subSize, size int64,
	proof [][]byte, root []byte, leafHash []byte,
) ([]byte, error) {
	if err := v.VerifyInclusionProof(subSize, size, proof, root, leafHash); err != nil {
		return nil, err
	}
	if subSize == 0 {
		return v.hasher.EmptyRoot(), nil
	}

	var res []byte
	inner := innerProofSize(subSize, size)
	for i, h := range proof {
		if i < inner && (subSize>>uint(i))&1 == 0 {
			continue
		}
		if res == nil {
			res = h
		} else {
			res = v.hasher.HashChildren(h, res)
		}
	}
	return res, nil
}

// decompInclProof breaks down inclusion proof for a leaf at the specified
// |index| in a tree of the specified |size| into 2 components. The splitting
// point between them is where paths to leaves |index| and |size-1| diverge.
// Returns lengths of the bottom and upper proof parts correspondingly. The sum
// of the two determines the correct length of the inclusion proof.
func decompInclProof(index, size int64) (int, int) {
	inner := innerProofSize(index, size)
	border := bits.OnesCount64(uint64(index) >> uint(inner))
	return inner, border
}
func innerProofSize(index, size int64) int {
	return bits.Len64(uint64(index ^ (size - 1)))
}

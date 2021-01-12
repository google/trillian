// Copyright 2016 Google LLC. All Rights Reserved.
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

package verifier

import (
	"bytes"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/tree"
)

// VerifyMapInclusionProof verifies that the passed in expectedRoot can be
// reconstructed correctly given the other parameters.
//
// The process is essentially the same as the inclusion proof checking for
// append-only logs, but adds support for nil/"default" proof nodes.
//
// Returns nil on a successful verification, and an error otherwise.
func VerifyMapInclusionProof(treeID int64, leaf *trillian.MapLeaf, expectedRoot []byte, proof [][]byte, h hashers.MapHasher) error {
	if got, want := len(leaf.Index)*8, h.BitLen(); got != want {
		return fmt.Errorf("index len: %d, want %d", got, want)
	}
	if got, want := len(proof), h.BitLen(); got != want {
		return fmt.Errorf("proof len: %d, want %d", got, want)
	}
	for i, element := range proof {
		if got, wanta, wantb := len(element), 0, h.Size(); got != wanta && got != wantb {
			return fmt.Errorf("proof[%d] len: %d, want %d or %d", i, got, wanta, wantb)
		}
	}

	leafHash := h.HashLeaf(treeID, leaf.Index, leaf.LeafValue)
	if len(leaf.LeafValue) == 0 && len(leaf.LeafHash) == 0 {
		// This is an empty value that has never been set, and so has a LeafHash of nil
		// (indicating that the effective hash value is h.HashEmpty(index, 0)).
		leafHash = nil
	}

	runningHash := leafHash
	nID := tree.NewNodeIDFromHash(leaf.Index)
	for height, sib := range nID.Siblings() {
		pElement := proof[height]

		// Since empty values are tied to a location and a level,
		// HashEmpty(leve1) != HashChildren(E0, E0).
		// Therefore we need to maintain an empty marker along the
		// proof path until the first non-empty element so we can call
		// HashEmpty once at the top of the empty branch.
		if len(runningHash) == 0 && len(pElement) == 0 {
			continue
		}
		// When we reach a level that has a neighbor, we compute the empty value
		// for the branch that we are on before combining it with the neighbor.
		if len(runningHash) == 0 && len(pElement) != 0 {
			depth := nID.PrefixLenBits - height
			emptyBranch := nID.MaskLeft(depth)
			runningHash = h.HashEmpty(treeID, emptyBranch.Path, height)
		}

		if len(runningHash) != 0 && len(pElement) == 0 {
			pElement = h.HashEmpty(treeID, sib.Path, height)
		}
		proofIsRightHandElement := nID.Bit(height) == 0
		if proofIsRightHandElement {
			runningHash = h.HashChildren(runningHash, pElement)
		} else {
			runningHash = h.HashChildren(pElement, runningHash)
		}
	}
	if len(runningHash) == 0 {
		depth := 0
		emptyBranch := nID.MaskLeft(depth)
		runningHash = h.HashEmpty(treeID, emptyBranch.Path, h.BitLen())
	}

	if got, want := runningHash, expectedRoot; !bytes.Equal(got, want) {
		return fmt.Errorf("calculated root: %x, want: %x", got, want)
	}
	return nil
}

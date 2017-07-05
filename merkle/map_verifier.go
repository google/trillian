// Copyright 2016 Google Inc. All Rights Reserved.
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
	"fmt"

	"github.com/google/trillian/merkle/hashers"
)

// VerifyMapInclusionProof verifies that the passed in expectedRoot can be
// reconstructed correctly given the other parameters.
//
// The process is essentially the same as the inclusion proof checking for
// append-only logs, but adds support for nil/"default" proof nodes.
//
// Returns nil on a successful verification, and an error otherwise.
func VerifyMapInclusionProof(treeID int64, index, leafHash, expectedRoot []byte, proof [][]byte, h hashers.MapHasher) error {
	if got, want := len(index)*8, h.BitLen(); got != want {
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

	runningHash := make([]byte, len(leafHash))
	copy(runningHash, leafHash)

	for level := 0; level < h.BitLen(); level++ {
		proofIsRightHandElement := bit(index, level) == 0
		pElement := proof[level]
		if len(pElement) == 0 {
			neighborIndex := Neighbor(index, level)
			pElement = h.HashEmpty(treeID, neighborIndex, level)
		}
		if proofIsRightHandElement {
			runningHash = h.HashChildren(runningHash, pElement)
		} else {
			runningHash = h.HashChildren(pElement, runningHash)
		}
	}

	if got, want := runningHash, expectedRoot; !bytes.Equal(got, want) {
		return fmt.Errorf("calculated root: %x, want \n%x", got, want)
	}
	return nil
}

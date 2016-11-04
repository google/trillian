package merkle

import (
	"bytes"
	"fmt"

	"github.com/google/trillian/storage"
)

// VerifyMapInclusionProof verifies that the passed in expectedRoot can be
// reconstructed correctly given the other parameters.
//
// The process is essentially the same as the inclusion proof checking for
// append-only logs, but adds support for nil/"default" proof nodes.
//
// Returns nil on a successful verification, and an error otherwise.
func VerifyMapInclusionProof(keyHash []byte, leafHash []byte, expectedRoot []byte, proof [][]byte, h MapHasher) error {
	hBits := h.Size() * 8

	if got, want := len(proof), hBits; got != want {
		return fmt.Errorf("invalid proof length %d, expected %d", got, want)
	}
	if got, want := len(keyHash)*8, hBits; got != want {
		return fmt.Errorf("invalid keyHash length %d, expected %d", got, want)
	}
	if got, want := len(leafHash)*8, hBits; got != want {
		return fmt.Errorf("invalid leafHash length %d, expected %d", got, want)
	}
	if got, want := len(expectedRoot)*8, hBits; got != want {
		return fmt.Errorf("invalid expectedRoot length %d, expected %d", got, want)
	}

	// TODO(al): Remove this dep on storage, since clients will want to use this code.
	nID := storage.NewNodeIDFromHash(keyHash)

	runningHash := make([]byte, len(leafHash))
	copy(runningHash, leafHash)

	for bit := 0; bit < hBits; bit++ {
		proofIsRightHandElement := nID.Bit(bit) == 0
		pElement := proof[bit]
		if len(pElement) == 0 {
			pElement = h.nullHashes[hBits-1-bit]
		}
		if got, want := len(pElement)*8, hBits; got != want {
			return fmt.Errorf("invalid proof: element has length %d, expected %d", got, want)
		}
		if proofIsRightHandElement {
			runningHash = h.HashChildren(runningHash, pElement)
		} else {
			runningHash = h.HashChildren(pElement, runningHash)
		}
	}

	if got, want := runningHash, expectedRoot; !bytes.Equal(got, want) {
		return fmt.Errorf("invalid proof; calculated roothash %v but expected %v", got, want)
	}
	return nil
}

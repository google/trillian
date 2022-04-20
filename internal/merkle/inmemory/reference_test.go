// Copyright 2022 Google LLC. All Rights Reserved.
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

package inmemory

import (
	"encoding/hex"
	"fmt"
	"math/bits"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/rfc6962"
	to "github.com/transparency-dev/merkle/testonly"
)

// refRootHash returns the root hash of a Merkle tree with the given entries.
// This is a reference implementation for cross-checking.
func refRootHash(entries [][]byte, hasher merkle.LogHasher) []byte {
	if len(entries) == 0 {
		return hasher.EmptyRoot()
	}
	if len(entries) == 1 {
		return hasher.HashLeaf(entries[0])
	}
	split := downToPowerOfTwo(uint64(len(entries)))
	return hasher.HashChildren(
		refRootHash(entries[:split], hasher),
		refRootHash(entries[split:], hasher))
}

// refInclusionProof returns the inclusion proof for the given leaf index in a
// Merkle tree with the given entries. This is a reference implementation for
// cross-checking.
func refInclusionProof(entries [][]byte, index uint64, hasher merkle.LogHasher) [][]byte {
	size := uint64(len(entries))
	if size == 1 || index >= size {
		return nil
	}
	split := downToPowerOfTwo(size)
	if index < split {
		return append(
			refInclusionProof(entries[:split], index, hasher),
			refRootHash(entries[split:], hasher))
	}
	return append(
		refInclusionProof(entries[split:], index-split, hasher),
		refRootHash(entries[:split], hasher))
}

// refConsistencyProof returns the consistency proof for the two tree sizes, in
// a Merkle tree with the given entries. This is a reference implementation for
// cross-checking.
func refConsistencyProof(entries [][]byte, size2, size1 uint64, hasher merkle.LogHasher, haveRoot1 bool) [][]byte {
	if size1 == 0 || size1 > size2 {
		return nil
	}
	// Consistency proof for two equal sizes is empty.
	if size1 == size2 {
		// Record the hash of this subtree if it's not the root for which the proof
		// was originally requested (which happens when size1 is a power of 2).
		if !haveRoot1 {
			return [][]byte{refRootHash(entries[:size1], hasher)}
		}
		return nil
	}

	// At this point: 0 < size1 < size2.
	split := downToPowerOfTwo(size2)
	if size1 <= split {
		// Root of size1 is in the left subtree of size2. Prove that the left
		// subtrees are consistent, and record the hash of the right subtree (only
		// present in size2).
		return append(
			refConsistencyProof(entries[:split], split, size1, hasher, haveRoot1),
			refRootHash(entries[split:], hasher))
	}

	// Root of size1 is at the same level as size2 root. Prove that the right
	// subtrees are consistent. The right subtree doesn't contain the root of
	// size1, so set haveRoot1 = false. Record the hash of the left subtree
	// (equal in both trees).
	return append(
		refConsistencyProof(entries[split:], size2-split, size1-split, hasher, false),
		refRootHash(entries[:split], hasher))
}

// downToPowerOfTwo returns the largest power of two smaller than x.
func downToPowerOfTwo(x uint64) uint64 {
	if x < 2 {
		panic("downToPowerOfTwo requires value >= 2")
	}
	return uint64(1) << (bits.Len64(x-1) - 1)
}

func TestDownToPowerOfTwo(t *testing.T) {
	for _, inOut := range [][2]uint64{
		{2, 1}, {7, 4}, {8, 4}, {63, 32}, {28937, 16384},
	} {
		if got, want := downToPowerOfTwo(inOut[0]), inOut[1]; got != want {
			t.Errorf("downToPowerOfTwo(%d): got %d, want %d", inOut[0], got, want)
		}
	}
}

func TestRefInclusionProof(t *testing.T) {
	for _, tc := range []struct {
		index uint64
		size  uint64
		want  [][]byte
	}{
		{index: 0, size: 1, want: nil},
		{index: 0, size: 2, want: [][]byte{
			hx("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"),
		}},
		{index: 1, size: 2, want: [][]byte{
			hx("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
		}},
		{index: 2, size: 3, want: [][]byte{
			hx("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"),
		}},
		{index: 1, size: 5, want: [][]byte{
			hx("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
			hx("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"),
			hx("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"),
		}},
		{index: 0, size: 8, want: [][]byte{
			hx("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"),
			hx("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"),
			hx("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"),
		}},
		{index: 5, size: 8, want: [][]byte{
			hx("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"),
			hx("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"),
			hx("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		}},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.index, tc.size), func(t *testing.T) {
			entries := to.LeafInputs()
			got := refInclusionProof(entries[:tc.size], tc.index, rfc6962.DefaultHasher)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("refInclusionProof: diff (-got +want)\n%s", diff)
			}
		})
	}
}

func TestRefConsistencyProof(t *testing.T) {
	for _, tc := range []struct {
		size1 uint64
		size2 uint64
		want  [][]byte
	}{
		{size1: 1, size2: 1, want: nil},
		{size1: 1, size2: 8, want: [][]byte{
			hx("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"),
			hx("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"),
			hx("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"),
		}},
		{size1: 2, size2: 5, want: [][]byte{
			hx("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"),
			hx("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"),
		}},
		{size1: 6, size2: 8, want: [][]byte{
			hx("0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a"),
			hx("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"),
			hx("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		}},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size1, tc.size2), func(t *testing.T) {
			entries := to.LeafInputs()
			got := refConsistencyProof(entries[:tc.size2], tc.size2, tc.size1, rfc6962.DefaultHasher, true)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("refConsistencyProof: diff (-got +want)\n%s", diff)
			}
		})
	}
}

// hx decodes a hex string or panics.
func hx(hs string) []byte {
	data, err := hex.DecodeString(hs)
	if err != nil {
		panic(fmt.Errorf("failed to decode test data: %s", hs))
	}
	return data
}

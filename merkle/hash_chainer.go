// Copyright 2018 Google Inc. All Rights Reserved.
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
	"math/bits"

	"github.com/google/trillian/merkle/hashers"
)

// hashChainer provides convenience methods for hashing subranges of Merkle
// Tree proofs to obtain (sub-)root hashes. Depending on how the path to a tree
// node relates to the query and/or tree borders, different methods are there.
//
// The family of Inner* chaining methods is for when the path is strictly below
// the right border of the tree. InnerRight* methods can be used for one-sided
// queries when the path goes along the query right border in addition to being
// below the right tree border, which is useful, for example, when computing
// hash of a tree sub-range rather than the full tree.
//
// Border* methods are for chaining proof hashes along (sub-)tree borders.
//
// *Open methods are for open-ended queries, i.e. when the query result should
// not include the boundary leaf.
//
// Note: There is no *Left in addition to *Right queries at the moment because
// Merkle Trees don't hash together nodes along left borders.
type hashChainer struct {
	hasher hashers.LogHasher
}

func (c hashChainer) chainInner(seed []byte, proof [][]byte, mask int64) []byte {
	for i, h := range proof {
		if (mask>>uint(i))&1 == 0 {
			seed = c.hasher.HashChildren(seed, h)
		} else {
			seed = c.hasher.HashChildren(h, seed)
		}
	}
	return seed
}

func (c hashChainer) chainInnerRight(seed []byte, proof [][]byte, mask int64) []byte {
	for i, h := range proof {
		if (mask>>uint(i))&1 == 1 {
			seed = c.hasher.HashChildren(h, seed)
		}
	}
	return seed
}

func (c hashChainer) chainInnerRightOpen(proof [][]byte, index int64) []byte {
	shift := bits.TrailingZeros64(uint64(index))
	if shift >= len(proof) {
		return nil
	}
	return c.chainInnerRight(proof[shift], proof[shift+1:], index>>uint(shift+1))
}

func (c hashChainer) chainBorderRight(seed []byte, proof [][]byte) []byte {
	for _, h := range proof {
		seed = c.hasher.HashChildren(h, seed)
	}
	return seed
}

func (c hashChainer) chainBorderRightOpen(seed []byte, proof [][]byte) []byte {
	if seed == nil {
		if len(proof) == 0 {
			return nil
		}
		return c.chainBorderRight(proof[0], proof[1:])
	}
	return c.chainBorderRight(seed, proof)
}

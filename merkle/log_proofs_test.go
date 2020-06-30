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

package merkle

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/merkle/compact"
)

func TestCalcInclusionProofNodeAddresses(t *testing.T) {
	// Expected inclusion proofs built by examination of the example 7 leaf tree
	// in RFC 6962:
	//
	//                hash              <== Level 3
	//               /    \
	//              /      \
	//             /        \
	//            /          \
	//           /            \
	//          k              l        <== Level 2
	//         / \            / \
	//        /   \          /   \
	//       /     \        /     \
	//      g       h      i      [ ]   <== Level 1
	//     / \     / \    / \    /
	//     a b     c d    e f    j      <== Level 0
	//     | |     | |    | |    |
	//     d0 d1   d2 d3  d4 d5  d6
	//
	// Remember that our storage node layers are always populated from the bottom
	// up, hence the gap at level 1, index 3 in the above picture.

	node := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, false)
	}
	rehash := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, true)
	}
	// These should all successfully compute the expected proof.
	for _, tc := range []struct {
		size    int64 // The requested past tree size.
		index   int64 // Leaf index in the requested tree.
		bigSize int64 // The current tree size.
		want    []NodeFetch
	}{
		// Small trees.
		{size: 1, index: 0, want: []NodeFetch{}},
		{size: 2, index: 0, want: []NodeFetch{node(0, 1)}},             // b
		{size: 2, index: 1, want: []NodeFetch{node(0, 0)}},             // a
		{size: 3, index: 1, want: []NodeFetch{node(0, 0), node(0, 2)}}, // a c

		// Tree of size 7.
		{size: 7, index: 0, want: []NodeFetch{
			node(0, 1), node(1, 1), node(2, 1)}}, // b h l
		{size: 7, index: 1, want: []NodeFetch{
			node(0, 0), node(1, 1), node(2, 1)}}, // a h l
		{size: 7, index: 2, want: []NodeFetch{
			node(0, 3), node(1, 0), node(2, 1)}}, // d g l
		{size: 7, index: 3, want: []NodeFetch{
			node(0, 2), node(1, 0), node(2, 1)}}, // c g l
		{size: 7, index: 4, want: []NodeFetch{
			node(0, 5), node(0, 6), node(2, 0)}}, // f j k
		{size: 7, index: 5, want: []NodeFetch{
			node(0, 4), node(0, 6), node(2, 0)}}, // e j k
		{size: 7, index: 6, want: []NodeFetch{
			node(1, 2), node(2, 0)}}, // i k

		// Smaller trees within a bigger stored tree.
		{size: 4, index: 2, bigSize: 7, want: []NodeFetch{
			node(0, 3), node(1, 0)}}, // d g
		{size: 5, index: 3, bigSize: 7, want: []NodeFetch{
			node(0, 2), node(1, 0), node(0, 4)}}, // c g e
		{size: 6, index: 3, bigSize: 7, want: []NodeFetch{
			node(0, 2), node(1, 0), node(1, 2)}}, // c g i
		{size: 6, index: 4, bigSize: 8, want: []NodeFetch{
			node(0, 5), node(2, 0)}}, // f k
		{size: 7, index: 1, bigSize: 8, want: []NodeFetch{
			node(0, 0), node(1, 1), rehash(0, 6), rehash(1, 2)}}, // a h l=hash(i,j)
		{size: 7, index: 3, bigSize: 8, want: []NodeFetch{
			node(0, 2), node(1, 0), rehash(0, 6), rehash(1, 2)}}, // c g l=hash(i,j)

		// Some rehashes in the middle of the returned list.
		{size: 15, index: 10, bigSize: 21, want: []NodeFetch{
			node(0, 11), node(1, 4), rehash(0, 14), rehash(1, 6), node(3, 0)}},
		{size: 31, index: 24, bigSize: 41, want: []NodeFetch{
			node(0, 25), node(1, 13),
			rehash(0, 30), rehash(1, 14),
			node(3, 2), node(4, 0)}},
		{size: 95, index: 81, bigSize: 111, want: []NodeFetch{
			node(0, 80), node(1, 41), node(2, 21),
			rehash(0, 94), rehash(1, 46), rehash(2, 22),
			node(4, 4), node(6, 0)}},
	} {
		bigSize := tc.size // Use the same tree size by default.
		if s := tc.bigSize; s != 0 {
			bigSize = s
		}
		t.Run(fmt.Sprintf("%d:%d:%d", tc.size, tc.index, bigSize), func(t *testing.T) {
			proof, err := CalcInclusionProofNodeAddresses(tc.size, tc.index, bigSize)
			if err != nil {
				t.Fatalf("CalcInclusionProofNodeAddresses: %v", err)
			}
			if diff := cmp.Diff(tc.want, proof); diff != "" {
				t.Errorf("paths mismatch:\n%v", diff)
			}
		})
	}
}

func TestCalcInclusionProofNodeAddressesBadRanges(t *testing.T) {
	for _, tc := range []struct {
		size    int64 // The requested past tree size.
		index   int64 // Leaf index in the requested tree.
		bigSize int64 // The current tree size.
	}{
		{size: 0, index: 0, bigSize: 0},
		{size: 0, index: 1, bigSize: 0},
		{size: 1, index: 0, bigSize: 0},
		{size: 1, index: 2, bigSize: 1},
		{size: 0, index: 3, bigSize: 0},
		{size: -1, index: 3, bigSize: -1},
		{size: 7, index: -1, bigSize: 7},
		{size: 7, index: 8, bigSize: 7},
		{size: 7, index: 3, bigSize: -7},
	} {
		t.Run(fmt.Sprintf("%d:%d:%d", tc.size, tc.index, tc.bigSize), func(t *testing.T) {
			_, err := CalcInclusionProofNodeAddresses(tc.size, tc.index, tc.bigSize)
			if err == nil {
				t.Fatal("accepted bad params")
			}
		})
	}
}

func TestCalcConsistencyProofNodeAddresses(t *testing.T) {
	// Expected consistency proofs built from the examples in RFC 6962. Again, in
	// our implementation node layers are filled from the bottom upwards.
	//
	//                hash5                         hash7
	//               /    \                        /    \
	//              /      \                      /      \
	//             /        \                    /        \
	//            /          \                  /          \
	//           /            \                /            \
	//          k             [ ]   =>        k              l
	//         / \            /              / \            / \
	//        /   \          /              /   \          /   \
	//       /     \        /              /     \        /     \
	//      g       h     [ ]             g       h      i      [ ]
	//     / \     / \    /              / \     / \    / \    /
	//     a b     c d    e              a b     c d    e f    j
	//     | |     | |    |              | |     | |    | |    |
	//     d0 d1   d2 d3  d4             d0 d1   d2 d3  d4 d5  d6
	//
	// For example, the consistency proof between tree size 5 and 7 consists of
	// nodes e, f, j, and k. The node j is taken instead of its missing parent.

	// These should compute the expected consistency proofs.
	for _, tc := range []struct {
		size1 int64
		size2 int64
		want  []NodeFetch
	}{
		{size1: 1, size2: 2, want: []NodeFetch{
			newNodeFetch(0, 1, false), // b
		}},
		{size1: 1, size2: 4, want: []NodeFetch{
			newNodeFetch(0, 1, false), // b
			newNodeFetch(1, 1, false), // h
		}},
		{size1: 2, size2: 8, want: []NodeFetch{
			newNodeFetch(1, 1, false), // h
			newNodeFetch(2, 1, false), // l
		}},
		{size1: 3, size2: 7, want: []NodeFetch{
			newNodeFetch(0, 2, false), // c
			newNodeFetch(0, 3, false), // d
			newNodeFetch(1, 0, false), // g
			newNodeFetch(2, 1, false), // l
		}},
		{size1: 4, size2: 7, want: []NodeFetch{
			newNodeFetch(2, 1, false), // l
		}},
		{size1: 5, size2: 7, want: []NodeFetch{
			newNodeFetch(0, 4, false), // e
			newNodeFetch(0, 5, false), // f
			newNodeFetch(0, 6, false), // j
			newNodeFetch(2, 0, false), // k
		}},
		{size1: 6, size2: 7, want: []NodeFetch{
			newNodeFetch(1, 2, false), // i
			newNodeFetch(0, 6, false), // j
			newNodeFetch(2, 0, false), // k
		}},
		{size1: 1, size2: 1, want: []NodeFetch{}},
		{size1: 2, size2: 2, want: []NodeFetch{}},
		{size1: 3, size2: 3, want: []NodeFetch{}},
		{size1: 4, size2: 4, want: []NodeFetch{}},
		{size1: 5, size2: 5, want: []NodeFetch{}},
		{size1: 7, size2: 7, want: []NodeFetch{}},
		{size1: 8, size2: 8, want: []NodeFetch{}},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size1, tc.size2), func(t *testing.T) {
			proof, err := CalcConsistencyProofNodeAddresses(tc.size1, tc.size2, tc.size2)
			if err != nil {
				t.Fatalf("CalcConsistencyProofNodeAddresses: %v", err)
			}
			if diff := cmp.Diff(tc.want, proof); diff != "" {
				t.Errorf("paths mismatch:\n%v", diff)
			}
		})
	}
}

func TestCalcConsistencyProofNodeAddressesBadInputs(t *testing.T) {
	// These should all fail to provide proofs.
	for _, tc := range []struct {
		size1 int64
		size2 int64
	}{
		{size1: 0, size2: -1},
		{size1: -10, size2: 0},
		{size1: -1, size2: -1},
		{size1: 0, size2: 0},
		{size1: 9, size2: 8},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size1, tc.size2), func(t *testing.T) {
			_, err := CalcConsistencyProofNodeAddresses(tc.size1, tc.size2, tc.size2)
			if err == nil {
				t.Fatal("accepted bad params")
			}
		})
	}
}

func TestInclusionSucceedsUpToTreeSize(t *testing.T) {
	const maxSize = 555
	for ts := 1; ts <= maxSize; ts++ {
		for i := ts; i < ts; i++ {
			if _, err := CalcInclusionProofNodeAddresses(int64(ts), int64(i), int64(ts)); err != nil {
				t.Errorf("CalcInclusionProofNodeAddresses(ts:%d, i:%d) = %v", ts, i, err)
			}
		}
	}
}

func TestConsistencySucceedsUpToTreeSize(t *testing.T) {
	const maxSize = 100
	for s1 := 1; s1 < maxSize; s1++ {
		for s2 := s1 + 1; s2 <= maxSize; s2++ {
			if _, err := CalcConsistencyProofNodeAddresses(int64(s1), int64(s2), int64(s2)); err != nil {
				t.Errorf("CalcConsistencyProofNodeAddresses(%d, %d) = %v", s1, s2, err)
			}
		}
	}
}

func newNodeFetch(level uint, index uint64, rehash bool) NodeFetch {
	return NodeFetch{ID: compact.NewNodeID(level, index), Rehash: rehash}
}

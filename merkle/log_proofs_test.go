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
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/compact"
)

// Expected consistency proofs built from the examples in RFC 6962. Again, in our implementation
// node layers are filled from the bottom upwards.
var (
	//                     hash1=g
	//                          / \
	//  hash0=a      =>         a b
	//        |                 | |
	//        d0               d0 d1
	expectedConsistencyProofFromSize1To2 = []NodeFetch{
		newNodeFetch(0, 1, false), // b
	}

	//  hash0=a      =>           hash1=k
	//        |                  /   \
	//        d0                /     \
	//                         /      \
	//                         /       \
	//                         g       h
	//                        / \     / \
	//                        a b     c d
	//                        | |     | |
	//                       d0 d1   d2 d3
	expectedConsistencyProofFromSize1To4 = []NodeFetch{
		newNodeFetch(0, 1, false), // b
		newNodeFetch(1, 1, false), // h
	}

	//                                             hash
	//                                            /    \
	//                                           /      \
	//                                          /        \
	//                                         /          \
	//                            =>          /            \
	//       hash0                           k              l
	//       / \                            / \            / \
	//      /   \                          /   \          /   \
	//     /     \                        /     \        /     \
	//     g     [ ]                     g       h      i      [ ]
	//    / \    /                      / \     / \    / \    /
	//    a b    c                      a b     c d    e f    j
	//    | |    |                      | |     | |    | |    |
	//   d0 d1   d2                     d0 d1   d2 d3  d4 d5  d6
	expectedConsistencyProofFromSize3To7 = []NodeFetch{
		newNodeFetch(0, 2, false), // c
		newNodeFetch(0, 3, false), // d
		newNodeFetch(1, 0, false), // g
		newNodeFetch(2, 1, false), // l
	}

	//                                             hash
	//                                            /    \
	//                                           /      \
	//                                          /        \
	//                                         /          \
	//                            =>          /            \
	//     hash1=k                           k              l
	//       /  \                           / \            / \
	//      /    \                         /   \          /   \
	//     /      \                       /     \        /     \
	//     g       h                     g       h      i      [ ]
	//    / \     / \                   / \     / \    / \    /
	//    a b     c d                   a b     c d    e f    j
	//    | |     | |                   | |     | |    | |    |
	//   d0 d1   d2 d3                  d0 d1   d2 d3  d4 d5  d6
	expectedConsistencyProofFromSize4To7 = []NodeFetch{
		newNodeFetch(2, 1, false), // l
	}

	//             hash2                           hash
	//             /  \                           /    \
	//            /    \                         /      \
	//           /      \                       /        \
	//          /        \                     /          \
	//         /          \       =>          /            \
	//        k            [ ]               k              l
	//       / \           /                / \            / \
	//      /   \         /                /   \          /   \
	//     /     \        |               /     \        /     \
	//    g       h       i              g       h      i      [ ]
	//   / \     / \     / \            / \     / \    / \    /
	//   a b     c d     e f            a b     c d    e f    j
	//   | |     | |     | |            | |     | |    | |    |
	//   d0 d1   d2 d3  d4 d5           d0 d1   d2 d3  d4 d5  d6
	expectedConsistencyProofFromSize6To7 = []NodeFetch{
		newNodeFetch(1, 2, false), // i
		newNodeFetch(0, 6, false), // j
		newNodeFetch(2, 0, false), // k
	}

	//                               hash8
	//                              /    \
	//                             /      \
	//                            /        \
	//                           /          \
	//              =>          /            \
	//                         k              l
	//                        / \            / \
	//                       /   \          /   \
	//  hash2=              /     \        /     \
	//     g               g       h      i      n
	//    / \             / \     / \    / \    / \
	//    a b             a b     c d    e f    j m
	//    | |             | |     | |    | |    | |
	//   d0 d1            d0 d1   d2 d3  d4 d5 d6 d7
	expectedConsistencyProofFromSize2To8 = []NodeFetch{
		newNodeFetch(1, 1, false), // h
		newNodeFetch(2, 1, false), // l
	}
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
	// These should all successfully compute the expected proof.
	for _, tc := range []struct {
		size  int64
		index int64
		want  []NodeFetch
	}{
		{size: 1, index: 0, want: nil},
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
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size, tc.index), func(t *testing.T) {
			proof, err := CalcInclusionProofNodeAddresses(tc.size, tc.index, tc.size)
			if err != nil {
				t.Fatalf("CalcInclusionProofNodeAddresses: %v", err)
			}
			comparePaths(t, "", proof, tc.want)
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
	// These should compute the expected consistency proofs.
	for _, testCase := range []struct {
		priorTreeSize int64
		treeSize      int64
		expectedProof []NodeFetch
	}{
		{1, 2, expectedConsistencyProofFromSize1To2},
		{1, 4, expectedConsistencyProofFromSize1To4},
		{6, 7, expectedConsistencyProofFromSize6To7},
		{3, 7, expectedConsistencyProofFromSize3To7},
		{4, 7, expectedConsistencyProofFromSize4To7},
		{2, 8, expectedConsistencyProofFromSize2To8},
		{1, 1, []NodeFetch{}},
		{2, 2, []NodeFetch{}},
		{3, 3, []NodeFetch{}},
		{4, 4, []NodeFetch{}},
		{5, 5, []NodeFetch{}},
		{7, 7, []NodeFetch{}},
		{8, 8, []NodeFetch{}},
	} {
		proof, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, testCase.treeSize)

		if err != nil {
			t.Fatalf("failed to calculate consistency proof from %d to %d: %v", testCase.priorTreeSize, testCase.treeSize, err)
		}

		comparePaths(t, fmt.Sprintf("c(%d, %d)", testCase.priorTreeSize, testCase.treeSize), proof, testCase.expectedProof)
	}
}

func TestCalcConsistencyProofNodeAddressesBadInputs(t *testing.T) {
	// These should all fail to provide proofs.
	for _, testCase := range []struct {
		priorTreeSize int64
		treeSize      int64
	}{
		{0, -1},
		{-10, 0},
		{-1, -1},
		{0, 0},
		{9, 8},
	} {
		_, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, testCase.treeSize)

		if err == nil {
			t.Fatalf("consistency path calculation accepted bad input: %v", testCase)
		}
	}
}

// TODO(pkalinnikov): Remove desc when all the tests use t.Run.
func comparePaths(t *testing.T, desc string, got, expected []NodeFetch) {
	if len(expected) != len(got) {
		t.Fatalf("%s: expected %d nodes in path but got %d: %v", desc, len(expected), len(got), got)
	}

	for i := 0; i < len(expected); i++ {
		if expected[i] != got[i] {
			t.Fatalf("%s: expected node %+v at position %d but got %+v", desc, expected[i], i, got[i])
		}
	}
}

func TestLastNodeWritten(t *testing.T) {
	for _, testCase := range []struct {
		ts     int64
		result string
	}{
		{3, "101"},
		{5, "1001"},
		{11, "10101"},
		{14, "11011"},
		{15, "11101"},
	} {
		str := ""
		for d := int64(len(testCase.result) - 1); d >= 0; d-- {
			if lastNodePresent(d, testCase.ts) {
				str += "1"
			} else {
				str += "0"
			}
		}

		if got, want := str, testCase.result; got != want {
			t.Errorf("lastNodeWritten(%d) got: %s, want: %s", testCase.ts, got, want)
		}
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

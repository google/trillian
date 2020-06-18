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

type auditPathTestData struct {
	treeSize     int64
	leafIndex    int64
	expectedPath []NodeFetch
}

type consistencyProofTestData struct {
	priorTreeSize int64
	treeSize      int64
	expectedProof []NodeFetch
}

var lastNodeWrittenVec = []struct {
	ts     int64
	result string
}{
	{3, "101"},
	{5, "1001"},
	{11, "10101"},
	{14, "11011"},
	{15, "11101"},
}

// Expected inclusion proof paths built by examination of the example 7 leaf tree in RFC 6962:
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
// When comparing with the document remember that our storage node layers are always
// populated from the bottom up, hence the gap at level 1, index 3 in the above picture.

var expectedPathSize7Index0 = []NodeFetch{ // from a
	newNodeFetch(0, 1, false), // b
	newNodeFetch(1, 1, false), // h
	newNodeFetch(2, 1, false), // l
}
var expectedPathSize7Index3 = []NodeFetch{ // from d
	newNodeFetch(0, 2, false), // c
	newNodeFetch(1, 0, false), // g
	newNodeFetch(2, 1, false), // l
}
var expectedPathSize7Index4 = []NodeFetch{ // from e
	newNodeFetch(0, 5, false), // f
	newNodeFetch(0, 6, false), // j
	newNodeFetch(2, 0, false), // k
}
var expectedPathSize7Index6 = []NodeFetch{ // from j
	newNodeFetch(1, 2, false), // i
	newNodeFetch(2, 0, false), // k
}

// Expected consistency proofs built from the examples in RFC 6962. Again, in our implementation
// node layers are filled from the bottom upwards.
var expectedConsistencyProofFromSize1To2 = []NodeFetch{
	//                     hash1=g
	//                          / \
	//  hash0=a      =>         a b
	//        |                 | |
	//        d0               d0 d1
	newNodeFetch(0, 1, false), // b
}
var expectedConsistencyProofFromSize1To4 = []NodeFetch{
	//
	//
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
	//
	//
	newNodeFetch(0, 1, false), // b
	newNodeFetch(1, 1, false), // h
}
var expectedConsistencyProofFromSize3To7 = []NodeFetch{
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
	newNodeFetch(0, 2, false), // c
	newNodeFetch(0, 3, false), // d
	newNodeFetch(1, 0, false), // g
	newNodeFetch(2, 1, false), // l
}
var expectedConsistencyProofFromSize4To7 = []NodeFetch{
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
	newNodeFetch(2, 1, false), // l
}
var expectedConsistencyProofFromSize6To7 = []NodeFetch{
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
	newNodeFetch(1, 2, false), // i
	newNodeFetch(0, 6, false), // j
	newNodeFetch(2, 0, false), // k
}
var expectedConsistencyProofFromSize2To8 = []NodeFetch{
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
	newNodeFetch(1, 1, false), // h
	newNodeFetch(2, 1, false), // l
}

// These should all successfully compute the expected path
var pathTests = []auditPathTestData{
	{1, 0, []NodeFetch{}},
	{7, 3, expectedPathSize7Index3},
	{7, 6, expectedPathSize7Index6},
	{7, 0, expectedPathSize7Index0},
	{7, 4, expectedPathSize7Index4},
}

// These should all fail
var pathTestBad = []auditPathTestData{
	{0, 1, []NodeFetch{}},
	{1, 2, []NodeFetch{}},
	{0, 3, []NodeFetch{}},
	{-1, 3, []NodeFetch{}},
	{7, -1, []NodeFetch{}},
	{7, 8, []NodeFetch{}},
}

// These should compute the expected consistency proofs
var consistencyTests = []consistencyProofTestData{
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
}

// These should all fail to provide proofs
var consistencyTestsBad = []consistencyProofTestData{
	{0, -1, []NodeFetch{}},
	{-10, 0, []NodeFetch{}},
	{-1, -1, []NodeFetch{}},
	{0, 0, []NodeFetch{}},
	{9, 8, []NodeFetch{}},
}

func TestCalcInclusionProofNodeAddresses(t *testing.T) {
	for _, testCase := range pathTests {
		path, err := CalcInclusionProofNodeAddresses(testCase.treeSize, testCase.leafIndex, testCase.treeSize)

		if err != nil {
			t.Fatalf("unexpected error calculating path %v: %v", testCase, err)
		}

		comparePaths(t, fmt.Sprintf("i(%d,%d)", testCase.leafIndex, testCase.treeSize), path, testCase.expectedPath)
	}
}

func TestCalcInclusionProofNodeAddressesBadRanges(t *testing.T) {
	for _, testCase := range pathTestBad {
		_, err := CalcInclusionProofNodeAddresses(testCase.treeSize, testCase.leafIndex, testCase.treeSize)

		if err == nil {
			t.Fatalf("incorrectly accepted bad params: %v", testCase)
		}
	}
}

func TestCalcConsistencyProofNodeAddresses(t *testing.T) {
	for _, testCase := range consistencyTests {
		proof, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, testCase.treeSize)

		if err != nil {
			t.Fatalf("failed to calculate consistency proof from %d to %d: %v", testCase.priorTreeSize, testCase.treeSize, err)
		}

		comparePaths(t, fmt.Sprintf("c(%d, %d)", testCase.priorTreeSize, testCase.treeSize), proof, testCase.expectedProof)
	}
}

func TestCalcConsistencyProofNodeAddressesBadInputs(t *testing.T) {
	for _, testCase := range consistencyTestsBad {
		_, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, testCase.treeSize)

		if err == nil {
			t.Fatalf("consistency path calculation accepted bad input: %v", testCase)
		}
	}
}

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
	for _, testCase := range lastNodeWrittenVec {
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

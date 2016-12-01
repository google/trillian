package merkle

import (
	"testing"

	"fmt"

	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
)

type bitLenTestData struct {
	input    int64
	expected int
}

type auditPathTestData struct {
	treeSize     int64
	leafIndex    int64
	expectedPath []storage.NodeID
}

type consistencyProofTestData struct {
	priorTreeSize int64
	treeSize      int64
	expectedProof []storage.NodeID
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

// For the path test tests at tree sizes up to this value
const testUpToTreeSize = 99

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

var expectedPathSize7Index0 = []storage.NodeID{ // from a
	testonly.MustCreateNodeIDForTreeCoords(0, 1, 64), // b
	testonly.MustCreateNodeIDForTreeCoords(1, 1, 64), // h
	testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), // l
}
var expectedPathSize7Index3 = []storage.NodeID{ // from d
	testonly.MustCreateNodeIDForTreeCoords(0, 2, 64), // c
	testonly.MustCreateNodeIDForTreeCoords(1, 0, 64), // g
	testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), // l
}
var expectedPathSize7Index4 = []storage.NodeID{ // from e
	testonly.MustCreateNodeIDForTreeCoords(0, 5, 64), // f
	testonly.MustCreateNodeIDForTreeCoords(0, 6, 64), // j
	testonly.MustCreateNodeIDForTreeCoords(2, 0, 64), // k
}
var expectedPathSize7Index6 = []storage.NodeID{ // from j
	testonly.MustCreateNodeIDForTreeCoords(1, 2, 64), // i
	testonly.MustCreateNodeIDForTreeCoords(2, 0, 64), // k
}

// Expected consistency proofs built from the examples in RFC 6962. Again, in our implementation
// node layers are filled from the bottom upwards.
var expectedConsistencyProofFromSize1To2 = []storage.NodeID{
	//                     hash1=g
	//                          / \
	//  hash0=a      =>         a b
	//        |                 | |
	//        d0               d0 d1
	testonly.MustCreateNodeIDForTreeCoords(0, 1, 64), // b
}
var expectedConsistencyProofFromSize1To4 = []storage.NodeID{
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
	testonly.MustCreateNodeIDForTreeCoords(0, 1, 64), // b
	testonly.MustCreateNodeIDForTreeCoords(1, 1, 64), // h
}
var expectedConsistencyProofFromSize3To7 = []storage.NodeID{
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
	testonly.MustCreateNodeIDForTreeCoords(0, 2, 64), // c
	testonly.MustCreateNodeIDForTreeCoords(0, 3, 64), // d
	testonly.MustCreateNodeIDForTreeCoords(1, 0, 64), // g
	testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), // l
}
var expectedConsistencyProofFromSize4To7 = []storage.NodeID{
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
	testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), // l
}
var expectedConsistencyProofFromSize6To7 = []storage.NodeID{
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
	testonly.MustCreateNodeIDForTreeCoords(1, 2, 64), // i
	testonly.MustCreateNodeIDForTreeCoords(0, 6, 64), // j
	testonly.MustCreateNodeIDForTreeCoords(2, 0, 64), // k
}

var bitLenTests = []bitLenTestData{{0, 0}, {1, 1}, {2, 2}, {3, 2}, {12, 4}}

// These should all successfully compute the expected path
var pathTests = []auditPathTestData{
	{1, 0, []storage.NodeID{}},
	{7, 3, expectedPathSize7Index3},
	{7, 6, expectedPathSize7Index6},
	{7, 0, expectedPathSize7Index0},
	{7, 4, expectedPathSize7Index4}}

// These should all fail
var pathTestBad = []auditPathTestData{
	{0, 1, []storage.NodeID{}},
	{1, 2, []storage.NodeID{}},
	{0, 3, []storage.NodeID{}},
	{-1, 3, []storage.NodeID{}},
	{7, -1, []storage.NodeID{}},
	{7, 8, []storage.NodeID{}},
}

// These should compute the expected consistency proofs
var consistencyTests = []consistencyProofTestData{
	{1, 2, expectedConsistencyProofFromSize1To2},
	{1, 4, expectedConsistencyProofFromSize1To4},
	{6, 7, expectedConsistencyProofFromSize6To7},
	{3, 7, expectedConsistencyProofFromSize3To7},
	{4, 7, expectedConsistencyProofFromSize4To7}}

// These should all fail to provide proofs
var consistencyTestsBad = []consistencyProofTestData{
	{0, -1, []storage.NodeID{}},
	{-10, 0, []storage.NodeID{}},
	{-1, -1, []storage.NodeID{}},
	{0, 0, []storage.NodeID{}},
	{9, 8, []storage.NodeID{}}}

func TestBitLen(t *testing.T) {
	for _, testCase := range bitLenTests {
		if got, expected := bitLen(testCase.input), testCase.expected; expected != got {
			t.Fatalf("expected %d for input %d but got %d", testCase.expected, testCase.input, got)
		}
	}
}

func TestCalcInclusionProofNodeAddresses(t *testing.T) {
	for _, testCase := range pathTests {
		path, err := CalcInclusionProofNodeAddresses(testCase.treeSize, testCase.leafIndex, 64)

		if err != nil {
			t.Fatalf("unexpected error calculating path %v: %v", testCase, err)
		}

		comparePaths(t, fmt.Sprintf("i(%d,%d)", testCase.leafIndex, testCase.treeSize), path, testCase.expectedPath)
	}
}

func TestCalcInclusionProofNodeAddressesBadRanges(t *testing.T) {
	for _, testCase := range pathTestBad {
		_, err := CalcInclusionProofNodeAddresses(testCase.treeSize, testCase.leafIndex, 64)

		if err == nil {
			t.Fatalf("incorrectly accepted bad params: %v", testCase)
		}
	}
}

func TestCalcInclusionProofNodeAddressesRejectsBadBitLen(t *testing.T) {
	_, err := CalcInclusionProofNodeAddresses(7, 3, -64)

	if err == nil {
		t.Fatal("incorrectly accepted -ve maxBitLen")
	}
}

func TestCalcConsistencyProofNodeAddresses(t *testing.T) {
	for _, testCase := range consistencyTests {
		proof, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, 64)

		if err != nil {
			t.Fatalf("failed to calculate consistency proof from %d to %d: %v", testCase.priorTreeSize, testCase.treeSize, err)
		}

		comparePaths(t, fmt.Sprintf("c(%d, %d)", testCase.priorTreeSize, testCase.treeSize), proof, testCase.expectedProof)
	}
}

func TestCalcConsistencyProofNodeAddressesBadInputs(t *testing.T) {
	for _, testCase := range consistencyTestsBad {
		_, err := CalcConsistencyProofNodeAddresses(testCase.priorTreeSize, testCase.treeSize, 64)

		if err == nil {
			t.Fatalf("consistency path calculation accepted bad input: %v", testCase)
		}
	}
}

func TestCalcConsistencyProofNodeAddressesRejectsBadBitLen(t *testing.T) {
	_, err := CalcConsistencyProofNodeAddresses(6, 7, -1)
	_, err2 := CalcConsistencyProofNodeAddresses(6, 7, 0)

	if err == nil || err2 == nil {
		t.Fatalf("consistency path calculation accepted bad bitlen: %v %v", err, err2)
	}
}

func comparePaths(t *testing.T, desc string, got, expected []storage.NodeID) {
	if len(expected) != len(got) {
		t.Fatalf("%s: expected %d nodes in path but got %d: %v", desc, len(expected), len(got), got)
	}

	for i := 0; i < len(expected); i++ {
		if !expected[i].Equivalent(got[i]) {
			t.Fatalf("%s: expected node %v (%v) at position %d but got %v (%v)", desc, expected[i], expected[i].CoordString(), i, got[i], got[i].CoordString())
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
	for ts := 1; ts < testUpToTreeSize; ts++ {
		for i := ts; i < ts; i++ {
			if _, err := CalcInclusionProofNodeAddresses(int64(ts), int64(i), 64); err != nil {
				t.Errorf("CalcInclusionProofNodeAddresses(ts:%d, i:%d) = %v", ts, i, err)
			}
		}
	}
}

func TestConsistencySucceedsUpToTreeSize(t *testing.T) {
	for s1 := 1; s1 < testUpToTreeSize; s1++ {
		for s2 := s1 + 1; s2 < testUpToTreeSize; s2++ {
			if _, err := CalcConsistencyProofNodeAddresses(int64(s1), int64(s2), 64); err != nil {
				t.Errorf("CalcConsistencyProofNodeAddresses(%d, %d) = %v", s1, s2, err)
			}
		}
	}
}
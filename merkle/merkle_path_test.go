package merkle

import (
	"testing"

	"github.com/google/trillian/storage"
)

type bitLenTestData struct {
	input    int64
	expected int
}

type calcPathTestData struct {
	treeSize     int64
	leafIndex    int64
	expectedPath []storage.NodeID
}

type consistencyProofTestData struct {
	priorTreeSize int64
	treeSize      int64
	expectedProof []storage.NodeID
}

// Expected paths built by examination of the example 7 leaf tree in RFC 6962. When comparing
// with the document remember that our storage node layers are always populated from the bottom up.
var expectedPathSize7Index0 = []storage.NodeID{storage.NewNodeIDForTreeCoords(0, 1, 64), storage.NewNodeIDForTreeCoords(1, 1, 64), storage.NewNodeIDForTreeCoords(2, 1, 64)}
var expectedPathSize7Index3 = []storage.NodeID{storage.NewNodeIDForTreeCoords(0, 2, 64), storage.NewNodeIDForTreeCoords(1, 0, 64), storage.NewNodeIDForTreeCoords(2, 1, 64)}
var expectedPathSize7Index4 = []storage.NodeID{storage.NewNodeIDForTreeCoords(0, 5, 64), storage.NewNodeIDForTreeCoords(0, 6, 64), storage.NewNodeIDForTreeCoords(2, 0, 64)}
var expectedPathSize7Index6 = []storage.NodeID{storage.NewNodeIDForTreeCoords(1, 2, 64), storage.NewNodeIDForTreeCoords(2, 0, 64)}

// Expected consistency proofs built from the examples in RFC 6962. Again, in our implementation
// node layers are filled from the bottom upwards.
var expectedConsistencyProofFromSize6To7 = []storage.NodeID{storage.NewNodeIDForTreeCoords(1, 2, 64), storage.NewNodeIDForTreeCoords(0, 6, 64), storage.NewNodeIDForTreeCoords(2, 0, 64)}
var expectedConsistencyProofFromSize3To7 = []storage.NodeID{storage.NewNodeIDForTreeCoords(0, 2, 64), storage.NewNodeIDForTreeCoords(0, 3, 64), storage.NewNodeIDForTreeCoords(1, 0, 64), storage.NewNodeIDForTreeCoords(2, 1, 64)}
var expectedConsistencyProofFromSize4To7 = []storage.NodeID{storage.NewNodeIDForTreeCoords(2, 1, 64)}

var bitLenTests = []bitLenTestData{{0, 0}, {1, 1}, {2, 2}, {3, 2}, {12, 4}}

// These should all successfully compute the expected path
var pathTests = []calcPathTestData{
	{1, 0, []storage.NodeID{}},
	{7, 3, expectedPathSize7Index3},
	{7, 6, expectedPathSize7Index6},
	{7, 0, expectedPathSize7Index0},
	{7, 4, expectedPathSize7Index4}}

// These should all fail
var pathTestBad = []calcPathTestData{
	{0, 1, []storage.NodeID{}},
	{1, 2, []storage.NodeID{}},
	{0, 3, []storage.NodeID{}},
	{-1, 3, []storage.NodeID{}},
	{7, -1, []storage.NodeID{}},
	{7, 8, []storage.NodeID{}},
}

// These should compute the expected consistency proofs
var consistencyTests = []consistencyProofTestData{
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

		comparePaths(t, path, testCase.expectedPath)
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
			t.Fatalf("failed to calculate inclusion proof from %d to %d: %v", testCase.priorTreeSize, testCase.treeSize, err)
		}

		comparePaths(t, proof, testCase.expectedProof)
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

func comparePaths(t *testing.T, got, expected []storage.NodeID) {
	if len(expected) != len(got) {
		t.Fatalf("expected %d nodes in path but got %d: %v", len(expected), len(got), got)
	}

	for i := 0; i < len(expected); i++ {
		if !expected[i].Equivalent(got[i]) {
			t.Fatalf("expected node %v at position %d but got %v", expected[i], i, got[i])
		}
	}
}

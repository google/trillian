package storage

import (
	"testing"
)

type bitLenTestData struct {
	input    int64
	expected int
}

type calcPathTestData struct {
	treeSize     int64
	leafIndex    int64
	expectedPath []NodeID
}

// Expected paths built by examination of the example 7 leaf tree in RFC 6962. When comparing
// with the document remember that our storage node layers are always populated from the bottom up.
var expectedPathSize7Index0 = []NodeID{NewNodeIDForTreeCoords(0, 1, 64), NewNodeIDForTreeCoords(1, 1, 64), NewNodeIDForTreeCoords(2, 1, 64)}
var expectedPathSize7Index3 = []NodeID{NewNodeIDForTreeCoords(0, 2, 64), NewNodeIDForTreeCoords(1, 0, 64), NewNodeIDForTreeCoords(2, 1, 64)}
var expectedPathSize7Index4 = []NodeID{NewNodeIDForTreeCoords(0, 5, 64), NewNodeIDForTreeCoords(0, 6, 64), NewNodeIDForTreeCoords(2, 0, 64)}
var expectedPathSize7Index6 = []NodeID{NewNodeIDForTreeCoords(1, 2, 64), NewNodeIDForTreeCoords(2, 0, 64)}

var bitLenTests = []bitLenTestData{{0, 0}, {1, 1}, {2, 2}, {3, 2}, {12, 4}}

// These should all successfully compute the expected path
var pathTests = []calcPathTestData{
	{7, 3, expectedPathSize7Index3},
	{7, 6, expectedPathSize7Index6},
	{7, 0, expectedPathSize7Index0},
	{7, 4, expectedPathSize7Index4}}

// These should all fail
var pathTestBad = []calcPathTestData{
	{0, 3, []NodeID{}},
	{-1, 3, []NodeID{}},
	{7, -1, []NodeID{}},
	{7, 8, []NodeID{}},
}

func TestBitLen(t *testing.T) {
	for _, testCase := range bitLenTests {
		if expected, got := testCase.expected, bitLen(testCase.input); expected != got {
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

		comparePaths(t, testCase.expectedPath, path)
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

func comparePaths(t *testing.T, expected, got []NodeID) {
	if len(expected) != len(got) {
		t.Fatalf("expected %d nodes in path but got %d: %v", len(expected), len(got), got)
	}

	for i := 0; i < len(expected); i++ {
		if !expected[i].Equivalent(got[i]) {
			t.Fatalf("expected node %v at position %d but got %v", expected[i], i, got[i])
		}
	}
}
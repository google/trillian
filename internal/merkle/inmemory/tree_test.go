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

package inmemory

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	_ "github.com/golang/glog"
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/rfc6962"
)

// TODO(pavelkalinnikov): Rewrite this file entirely.

var fuzzTestSize = int64(256)

// This is the hash of an empty string
var emptyTreeHashValue = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Inputs to the reference tree, which has eight leaves.
var leafInputs = []string{
	"", "00", "10", "2021", "3031", "40414243",
	"5051525354555657", "606162636465666768696a6b6c6d6e6f",
}

// Incremental roots from building the reference tree from inputs leaf-by-leaf.
// Generated from ReferenceMerkleTreeHash in C++.
var rootsAtSize = []string{
	"6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
	"fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125",
	"aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77",
	"d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
	"4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4",
	"76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef",
	"ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c",
	"5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328",
}

// Some paths for the reference tree.
type pathTestVector struct {
	leaf       uint64
	snapshot   uint64
	testVector []string
}

// Generated from C++ ReferenceMerklePath, not the Go one so we can verify
// that they are both producing the same paths in a sanity test.
var testPaths = []pathTestVector{
	{0, 1, []string{}},
	{1, 2, []string{"6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"}},
	{
		0,
		8,
		[]string{
			"96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7",
			"5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
			"6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4",
		},
	},
	{
		5,
		8,
		[]string{
			"bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b",
			"ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0",
			"d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
		},
	},
	{
		2,
		3,
		[]string{"fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"},
	},
	{
		1,
		5,
		[]string{
			"6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
			"5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
			"bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b",
		},
	},
}

type proofTestVector struct {
	snapshot1 uint64
	snapshot2 uint64
	proof     []string
}

// Generated from ReferenceSnapshotConsistency in C++ version.
var testProofs = []proofTestVector{
	{1, 1, []string{}},
	{1, 8, []string{
		"96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7",
		"5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
		"6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4",
	}},
	{6, 8, []string{
		"0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a",
		"ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0",
		"d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
	}},
	{2, 5, []string{
		"5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
		"bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b",
	}},
}

func decodeHexStringOrPanic(hs string) []byte {
	data, err := hex.DecodeString(hs)
	if err != nil {
		panic(fmt.Errorf("failed to decode test data: %s", hs))
	}

	return data
}

func makeEmptyTree() *Tree {
	return New(rfc6962.DefaultHasher)
}

func makeFuzzTestData() [][]byte {
	var data [][]byte

	for s := int64(0); s < fuzzTestSize; s++ {
		data = append(data, make([]byte, 1))
		data[s][0] = byte(s)
	}

	return data
}

func getRootAsString(mt *Tree, size uint64) string {
	if size > mt.Size() {
		// Doesn't matter what this is as long as it could never be a valid
		// hex encoding of a hash
		return "<nil>"
	}
	return hex.EncodeToString(mt.HashAt(size))
}

// REFERENCE IMPLEMENTATIONS

// Get the largest power of two smaller than i.
func downToPowerOfTwo(i uint64) uint64 {
	if i < 2 {
		panic(errors.New("requested downToPowerOf2 for value < 2"))
	}

	// Find the smallest power of two greater than or equal to i. We
	// know i > 2
	split := uint64(2)

	for split < i {
		split <<= 1
	}

	// Get the largest power of two smaller than i.
	return split >> 1
}

// Reference implementation of Merkle hash, for cross-checking.
func referenceMerkleTreeHash(inputs [][]byte, treehasher merkle.LogHasher) ([]byte, error) {
	if len(inputs) == 0 {
		return treehasher.EmptyRoot(), nil
	}
	if len(inputs) == 1 {
		return treehasher.HashLeaf(inputs[0]), nil
	}

	split := downToPowerOfTwo(uint64(len(inputs)))

	lhs, err := referenceMerkleTreeHash(inputs[:split], treehasher)
	if err != nil {
		return nil, err
	}
	rhs, err := referenceMerkleTreeHash(inputs[split:], treehasher)
	if err != nil {
		return nil, err
	}
	return treehasher.HashChildren(lhs, rhs), nil
}

// Reference implementation of Merkle paths. Path from leaf to root,
// excluding the leaf and root themselves.
func referenceMerklePath(inputs [][]byte, leaf uint64, treehasher merkle.LogHasher) ([][]byte, error) {
	var path [][]byte

	inputLen := uint64(len(inputs))
	if leaf >= inputLen {
		return path, nil
	}

	if inputLen == 1 {
		return path, nil
	}

	split := downToPowerOfTwo(inputLen)

	var subpath [][]byte

	if leaf < split {
		s, err := referenceMerklePath(inputs[:split], leaf, treehasher)
		if err != nil {
			return nil, err
		}
		subpath = s
		path = append(path, subpath...)
		refHash, err := referenceMerkleTreeHash(inputs[split:], treehasher)
		if err != nil {
			return nil, err
		}
		path = append(path, refHash)
	} else {
		s, err := referenceMerklePath(inputs[split:], leaf-split, treehasher)
		if err != nil {
			return nil, err
		}
		subpath = s
		path = append(path, subpath...)
		refHash, err := referenceMerkleTreeHash(inputs[:split], treehasher)
		if err != nil {
			return nil, err
		}
		path = append(path, refHash)
	}

	return path, nil
}

// Reference implementation of snapshot consistency.
// Call with haveRoot1 = true.
func referenceSnapshotConsistency(inputs [][]byte, snapshot2 uint64,
	snapshot1 uint64, treehasher merkle.LogHasher, haveRoot1 bool) ([][]byte, error) {
	var proof [][]byte

	if snapshot1 == 0 || snapshot1 > snapshot2 {
		return proof, nil
	}

	if snapshot1 == snapshot2 {
		// Consistency proof for two equal subtrees is empty.
		if !haveRoot1 {
			// Record the hash of this subtree unless it's the root for which
			// the proof was originally requested. (This happens when the snapshot1
			// tree is balanced.)
			refHash, err := referenceMerkleTreeHash(inputs[:snapshot1], treehasher)
			if err != nil {
				return nil, err
			}
			proof = append(proof, refHash)
		}
		return proof, nil
	}

	// 0 < snapshot1 < snapshot2
	split := downToPowerOfTwo(snapshot2)

	var subproof [][]byte
	if snapshot1 <= split {
		// Root of snapshot1 is in the left subtree of snapshot2.
		// Prove that the left subtrees are consistent.
		s, err := referenceSnapshotConsistency(inputs[:split], split, snapshot1, treehasher, haveRoot1)
		if err != nil {
			return nil, err
		}
		subproof = s
		proof = append(proof, subproof...)
		// Record the hash of the right subtree (only present in snapshot2).
		h, err := referenceMerkleTreeHash(inputs[split:], treehasher)
		if err != nil {
			return nil, err
		}
		proof = append(proof, h)
	} else {
		// Snapshot1 root is at the same level as snapshot2 root.
		// Prove that the right subtrees are consistent. The right subtree
		// doesn't contain the root of snapshot1, so set haveRoot1 = false.
		s, err := referenceSnapshotConsistency(inputs[split:], snapshot2-split, snapshot1-split, treehasher, false)
		if err != nil {
			return nil, err
		}
		subproof = s

		proof = append(proof, subproof...)
		// Record the hash of the left subtree (equal in both trees).
		refHash, err := referenceMerkleTreeHash(inputs[:split], treehasher)
		if err != nil {
			return nil, err
		}
		proof = append(proof, refHash)
	}
	return proof, nil
}

func TestEmptyTreeIsEmpty(t *testing.T) {
	mt := makeEmptyTree()

	if size := mt.Size(); size != 0 {
		t.Errorf("Empty tree had leaves: %d", size)
	}
}

func TestEmptyTreeHash(t *testing.T) {
	actual := makeEmptyTree().Hash()
	actualStr := hex.EncodeToString(actual)

	if actualStr != emptyTreeHashValue {
		t.Errorf("Unexpected empty tree hash: %s", actualStr)
	}
}

func validateTree(mt *Tree, l uint64, t *testing.T) {
	if got, want := mt.Size(), l+1; got != want {
		t.Errorf("Incorrect leaf count %d, expecting %d", got, want)
	}

	if got, want := getRootAsString(mt, l+1), rootsAtSize[l]; got != want {
		t.Errorf("Incorrect root %d, got %s", l, got)
	}

	if got, want := getRootAsString(mt, 0), emptyTreeHashValue; got != want {
		t.Errorf("Incorrect root(0) %d, got %s", l, got)
	}

	for j := uint64(0); j <= l; j++ {
		if got, want := getRootAsString(mt, j+1), rootsAtSize[j]; got != want {
			t.Errorf("Incorrect root %d, %d, got %s", l, j, got)
		}
	}

	for k := l + 1; k <= 8; k++ {
		if got, want := getRootAsString(mt, k+1), "<nil>"; got != want {
			t.Errorf("Got root for missing leaf %d, %d, %s", l, k, got)
		}
	}
}

func TestBuildTreeBuildOneAtATime(t *testing.T) {
	mt := makeEmptyTree()

	// Add to the tree, checking after each leaf
	for l := uint64(0); l < 8; l++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[l]))
		validateTree(mt, l, t)
	}
}

func TestBuildTreeBuildAllAtOnce(t *testing.T) {
	mt := makeEmptyTree()

	for l := 0; l < 3; l++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[l]))
	}

	// Check the intermediate state
	validateTree(mt, 2, t)

	for l := 3; l < 8; l++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[l]))
	}

	// Check the final state
	validateTree(mt, 7, t)
}

func TestBuildTreeBuildTwoChunks(t *testing.T) {
	mt := makeEmptyTree()

	// Add to the tree, checking after each leaf
	for l := 0; l < 8; l++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[l]))
	}

	validateTree(mt, 7, t)
}

func TestDownToPowerOfTwoSanity(t *testing.T) {
	if downToPowerOfTwo(7) != 4 {
		t.Errorf("Down to power of 2 returned 7 -> %d", downToPowerOfTwo(7))
	}
	if downToPowerOfTwo(8) != 4 {
		t.Errorf("Down to power of 2 returned 8 -> %d", downToPowerOfTwo(8))
	}
	if downToPowerOfTwo(63) != 32 {
		t.Errorf("Down to power of 2 returned 63 -> %d", downToPowerOfTwo(63))
	}
	if downToPowerOfTwo(28973) != 16384 {
		t.Errorf("Down to power of 2 returned 63 -> %d", downToPowerOfTwo(28973))
	}
}

func TestReferenceMerklePathSanity(t *testing.T) {
	var data [][]byte

	mt := makeEmptyTree()

	for s := 0; s < 8; s++ {
		data = append(data, decodeHexStringOrPanic(leafInputs[s]))
	}

	for _, path := range testPaths {
		referencePath, err := referenceMerklePath(data[:path.snapshot], path.leaf, mt.hasher)
		if err != nil {
			t.Fatalf("referenceMerklePath(): %v", err)
		}

		if len(referencePath) != len(path.testVector) {
			t.Errorf("Mismatched path length: %d, %d: %v %v",
				len(referencePath), len(path.testVector), path, referencePath)
		}

		for i := 0; i < len(path.testVector); i++ {
			if !bytes.Equal(referencePath[i], decodeHexStringOrPanic(path.testVector[i])) {
				t.Errorf("Path mismatch: %s, %s", hex.EncodeToString(referencePath[i]),
					path.testVector[i])
			}
		}
	}
}

func TestMerkleTreeRootFuzz(t *testing.T) {
	data := makeFuzzTestData()

	for treeSize := int64(1); treeSize <= fuzzTestSize; treeSize++ {
		mt := makeEmptyTree()

		for l := int64(0); l < treeSize; l++ {
			mt.AppendData(data[l])
		}

		// Since the tree is evaluated lazily, the order of queries is significant.
		// Generate a random sequence of 8 queries for each tree.
		for j := int64(0); j < 8; j++ {
			// A snapshot in the range 0...tree_size.
			snapshot := uint64(rand.Int63n(treeSize + 1))

			h1 := mt.HashAt(snapshot)
			h2, err := referenceMerkleTreeHash(data[:snapshot], mt.hasher)
			if err != nil {
				t.Fatalf("referenceMerkleTreeHash(): %v", err)
			}

			if !bytes.Equal(h1, h2) {
				t.Errorf("Mismatched hash: %x, %x", h1, h2)
			}
		}
	}
}

// Make random path queries and check against the reference implementation.
func TestMerkleTreePathFuzz(t *testing.T) {
	data := makeFuzzTestData()

	for treeSize := int64(1); treeSize <= fuzzTestSize; treeSize++ {
		// mt := makeLoggingEmptyTree(t)
		mt := makeEmptyTree()

		for l := int64(0); l < treeSize; l++ {
			mt.AppendData(data[l])
		}

		// Since the tree is evaluated lazily, the order of queries is significant.
		// Generate a random sequence of 8 queries for each tree.
		for j := 0; j < 8; j++ {
			// A snapshot in the range 1..treeSize.
			snapshot := uint64(rand.Int63n(treeSize)) + 1
			// A leaf in the range 0..snapshot-1.
			leaf := uint64(rand.Int63n(int64(snapshot)))

			p1, err := mt.InclusionProof(leaf, snapshot)
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}

			p2, err := referenceMerklePath(data[:snapshot], leaf, mt.hasher)
			if err != nil {
				t.Fatalf("referenceMerklePath(): %v", err)
			}

			if len(p1) != len(p2) {
				t.Errorf("Different path lengths %v, %v", p1, p2)
			} else {
				for i := 0; i < len(p1); i++ {
					if !bytes.Equal(p1[i], p2[i]) {
						t.Errorf("Mismatched hash %d %d %d: %v, %v", snapshot, leaf, i,
							p1[i], p2[i])
					}
				}
			}
		}
	}
}

// Make random proof queries and check against the reference implementation.
func TestMerkleTreeConsistencyFuzz(t *testing.T) {
	data := makeFuzzTestData()

	for treeSize := int64(1); treeSize <= fuzzTestSize; treeSize++ {
		mt := makeEmptyTree()

		for l := int64(0); l < treeSize; l++ {
			mt.AppendData(data[l])
		}

		// Since the tree is evaluated lazily, the order of queries is significant.
		// Generate a random sequence of 8 queries for each tree.
		for j := 0; j < 8; j++ {
			// A snapshot in the range 0... length.
			snapshot2 := uint64(rand.Int63n(treeSize + 1))
			// A snapshot in the range 0... snapshot.
			snapshot1 := uint64(rand.Int63n(int64(snapshot2) + 1))

			c1, err := mt.ConsistencyProof(snapshot1, snapshot2)
			if err != nil {
				t.Fatalf("ConsistencyProof: %v", err)
			}
			c2, err := referenceSnapshotConsistency(data[:snapshot2], snapshot2, snapshot1, mt.hasher, true)
			if err != nil {
				t.Fatalf("referenceSnapshotConsistency(): %v", err)
			}

			if len(c1) != len(c2) {
				t.Errorf("Different proof lengths: %d %d %d", treeSize, snapshot2,
					snapshot1)
			}

			for i := 0; i < len(c1); i++ {
				if !bytes.Equal(c1[i], c2[i]) {
					t.Errorf("Different proof: %d %d %d %d, %s, %s", treeSize,
						snapshot2, snapshot1, i, hex.EncodeToString(c1[i]),
						hex.EncodeToString(c2[i]))
				}
			}
		}
	}
}

func TestMerkleTreePathBuildOnce(t *testing.T) {
	// First tree: build in one go.
	mt := makeEmptyTree()

	for i := 0; i < 8; i++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[i]))
	}

	if size := mt.Size(); size != 8 {
		t.Fatalf("8 leaves added but tree size is %d", size)
	}

	hash := mt.Hash()
	if got, want := hash, decodeHexStringOrPanic(rootsAtSize[7]); !bytes.Equal(got, want) {
		t.Fatalf("Got unexpected root hash: %x %x", got, want)
	}

	if proof, err := mt.InclusionProof(8, 8); err == nil {
		t.Fatalf("Obtained a path for non existent leaf 9: %v", proof)
	}

	for i := 0; i < 6; i++ {
		p1, err := mt.InclusionProof(testPaths[i].leaf, testPaths[i].snapshot)
		if err != nil {
			t.Fatalf("InclusionProof: %v", err)
		}

		p2 := append([]string{}, testPaths[i].testVector...)

		if len(p1) != len(p2) {
			t.Errorf("Different path lengths %d %d", len(p1), len(p2))
			t.FailNow()
		}

		for j := range p2 {
			if got, want := p1[j], decodeHexStringOrPanic(testPaths[i].testVector[j]); !bytes.Equal(got, want) {
				t.Errorf("Path mismatch: got: %v want: %v", got, want)
			}
		}
	}
}

func TestMerkleTreePathBuildIncrementally(t *testing.T) {
	// Second tree: build incrementally.
	// First tree: build in one go.
	mt := makeEmptyTree()

	for i := 0; i < 8; i++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[i]))
	}

	mt2 := makeEmptyTree()

	for i := uint64(0); i < 8; i++ {
		mt2.AppendData(decodeHexStringOrPanic(leafInputs[i]))

		for j := uint64(0); j < i+1; j++ {
			p1, err := mt.InclusionProof(j, i+1)
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}
			p2, err := mt2.InclusionProof(j, mt2.Size())
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}

			if len(p1) != len(p2) {
				t.Errorf("Different path lengths %d %d", len(p1), len(p2))
				t.FailNow()
			}

			for j := 0; j < len(p2); j++ {
				if !bytes.Equal(p1[j], p2[j]) {
					t.Errorf("Path mismatch: %s %s", hex.EncodeToString(p1[j]),
						hex.EncodeToString(p2[j]))
				}
			}
		}

		for k := i + 2; k <= 9; k++ {
			if proof, err := mt.InclusionProof(k, i+1); err == nil {
				t.Errorf("Got non empty path unexpectedly: %d %d %d", i, k, len(proof))
			}
		}
	}
}

func TestProofConsistencyTestVectors(t *testing.T) {
	mt := makeEmptyTree()

	for i := 0; i < 8; i++ {
		mt.AppendData(decodeHexStringOrPanic(leafInputs[i]))
	}

	if size := mt.Size(); size != 8 {
		t.Fatalf("8 leaves added but tree size is %d", size)
	}

	hash := mt.Hash()
	if got, want := hash, decodeHexStringOrPanic(rootsAtSize[7]); !bytes.Equal(got, want) {
		t.Fatalf("Got unexpected root hash: %x %x", got, want)
	}

	for i := 0; i < 4; i++ {
		p1, err := mt.ConsistencyProof(testProofs[i].snapshot1, testProofs[i].snapshot2)
		if err != nil {
			t.Fatalf("ConsistencyProof: %v", err)
		}

		p2 := append([]string{}, testProofs[i].proof...)

		if len(p1) != len(p2) {
			t.Errorf("Different proof lengths %d %d", len(p1), len(p2))
			t.FailNow()
		}

		for j := 0; j < len(p2); j++ {
			if got, want := p1[j], decodeHexStringOrPanic(testProofs[i].proof[j]); !bytes.Equal(got, want) {
				t.Errorf("Path mismatch: got: %v want: %v", got, want)
			}
		}
	}
}

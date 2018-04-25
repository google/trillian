// Copyright 2017 Google Inc. All Rights Reserved.
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
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
)

type logProofTestVector struct {
	h []byte
	l int
}

type inclusionProofTestVector struct {
	leaf, snapshot, proofLength int64
	proof                       []logProofTestVector
}

type consistencyTestVector struct {
	snapshot1, snapshot2, proofLen int64
	proof                          [3]logProofTestVector
}

var (
	sha256SomeHash      = dh("abacaba000000000000000000000000000000000000000000060061e00123456")
	sha256EmptyTreeHash = dh("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	inclusionProofs     = []inclusionProofTestVector{
		{0, 0, 0, []logProofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1, 1, 0, []logProofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1,
			8,
			3,
			[]logProofTestVector{
				{dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"), 32}}},
		{6,
			8,
			3,
			[]logProofTestVector{
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32},
				{dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"), 32},
				{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32}}},
		{3,
			3,
			1,
			[]logProofTestVector{
				{dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"), 32},
				{dh(""), 0},
				{dh(""), 0}}},
		{2,
			5,
			3,
			[]logProofTestVector{
				{dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32}},
		}}
	consistencyProofs = []consistencyTestVector{
		{1, 1, 0, [3]logProofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1,
			8,
			3,
			[3]logProofTestVector{
				{dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"), 32}}},
		{6,
			8,
			3,
			[3]logProofTestVector{
				{dh("0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a"), 32},
				{dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"), 32},
				{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32}}},
		{2,
			5,
			2,
			[3]logProofTestVector{
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32},
				{dh(""), 0}}},
	}
	roots = []logProofTestVector{
		{dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"), 32},
		{dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"), 32},
		{dh("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77"), 32},
		{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32},
		{dh("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4"), 32},
		{dh("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef"), 32},
		{dh("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c"), 32},
		{dh("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328"), 32}}
	leaves = []logProofTestVector{
		{dh(""), 0},
		{dh("00"), 1},
		{dh("10"), 1},
		{dh("2021"), 2},
		{dh("3031"), 2},
		{dh("40414243"), 4},
		{dh("5051525354555657"), 8},
		{dh("606162636465666768696a6b6c6d6e6f"), 16},
	}
)

// inclusionProbe is a parameter set for inclusion proof verification.
type inclusionProbe struct {
	leafIndex int64
	treeSize  int64
	root      []byte
	leafHash  []byte
	proof     [][]byte

	desc string
}

// consistencyProbe is a parameter set for consistency proof verification.
type consistencyProbe struct {
	snapshot1 int64
	snapshot2 int64
	root1     []byte
	root2     []byte
	proof     [][]byte

	desc string
}

func corruptInclVerification(leafIndex, treeSize int64, proof [][]byte, root, leafHash []byte) []inclusionProbe {
	ret := []inclusionProbe{
		// Wrong leaf index.
		{leafIndex - 1, treeSize, root, leafHash, proof, "leafIndex - 1"},
		{leafIndex + 1, treeSize, root, leafHash, proof, "leafIndex + 1"},
		{leafIndex ^ 2, treeSize, root, leafHash, proof, "leafIndex ^ 2"},
		// Wrong tree height.
		{leafIndex, treeSize * 2, root, leafHash, proof, "treeSize * 2"},
		{leafIndex, treeSize / 2, root, leafHash, proof, "treeSize / 2"},
		// Wrong leaf or root.
		{leafIndex, treeSize, root, []byte("WrongLeaf"), proof, "wrong leaf"},
		{leafIndex, treeSize, sha256EmptyTreeHash, leafHash, proof, "empty root"},
		{leafIndex, treeSize, sha256SomeHash, leafHash, proof, "random root"},
		// Add garbage at the end.
		{leafIndex, treeSize, root, leafHash, append(proof, []byte{}), "trailing garbage"},
		{leafIndex, treeSize, root, leafHash, append(proof, root), "trailing root"},
		// Add garbage at the front.
		{leafIndex, treeSize, root, leafHash, append([][]byte{{}}, proof...), "preceding garbage"},
		{leafIndex, treeSize, root, leafHash, append([][]byte{root}, proof...), "preceding root"},
	}
	ln := len(proof)

	// Modify single bit in an element of the proof.
	for i := 0; i < ln; i++ {
		wrongProof := append([][]byte(nil), proof...)
		wrongProof[i][0] ^= 8
		desc := fmt.Sprintf("modified proof[%d] bit 3", i)
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, wrongProof, desc})
	}

	if ln > 0 {
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, proof[:ln-1], "removed component"})
	}
	if ln > 1 {
		wrongProof := append([][]byte{proof[0], sha256SomeHash}, proof[1:]...)
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, wrongProof, "inserted component"})
	}

	return ret
}

func corruptConsVerification(snapshot1, snapshot2 int64, root1, root2 []byte, proof [][]byte) []consistencyProbe {
	ln := len(proof)
	ret := []consistencyProbe{
		// Wrong snapshot index.
		{snapshot1 - 1, snapshot2, root1, root2, proof, "snapshot1 - 1"},
		{snapshot1 + 1, snapshot2, root1, root2, proof, "snapshot1 + 1"},
		{snapshot1 ^ 2, snapshot2, root1, root2, proof, "snapshot1 ^ 2"},
		// Wrong tree height.
		{snapshot1, snapshot2 * 2, root1, root2, proof, "snapshot2 * 2"},
		{snapshot1, snapshot2 / 2, root1, root2, proof, "snapshot2 / 2"},
		// Wrong root.
		{snapshot1, snapshot2, []byte("WrongRoot"), root2, proof, "wrong root1"},
		{snapshot1, snapshot2, root1, []byte("WrongRoot"), proof, "wrong root2"},
		{snapshot1, snapshot2, root2, root1, proof, "swapped roots"},
		// Empty proof.
		{snapshot1, snapshot2, root1, root2, [][]byte{}, "empty proof"},
		// Add garbage at the end.
		{snapshot1, snapshot2, root1, root2, append(proof, []byte{}), "trailing garbage"},
		{snapshot1, snapshot2, root1, root2, append(proof, root1), "trailing root1"},
		{snapshot1, snapshot2, root1, root2, append(proof, root2), "trailing root2"},
		// Add garbage at the front.
		{snapshot1, snapshot2, root1, root2, append([][]byte{{}}, proof...), "preceding garbage"},
		{snapshot1, snapshot2, root1, root2, append([][]byte{root1}, proof...), "preceding root1"},
		{snapshot1, snapshot2, root1, root2, append([][]byte{root2}, proof...), "preceding root2"},
		{snapshot1, snapshot2, root1, root2, append([][]byte{proof[0]}, proof...), "preceding proof[0]"},
	}

	// Remove a node from the end.
	if ln > 0 {
		ret = append(ret, consistencyProbe{snapshot1, snapshot2, root1, root2, proof[:ln-1], "truncated proof"})
	}

	// Modify single bit in an element of the proof.
	for i := 0; i < ln; i++ {
		wrongProof := append([][]byte(nil), proof...)
		wrongProof[i][0] ^= 16
		desc := fmt.Sprintf("modified proof[%d] bit 4", i)
		ret = append(ret, consistencyProbe{snapshot1, snapshot2, root1, root2, wrongProof, desc})
	}

	return ret
}

func verifierCheck(v *LogVerifier, leafIndex, treeSize int64, proof [][]byte, root, leafHash []byte) error {
	// Verify original inclusion proof.
	got, err := v.RootFromInclusionProof(leafIndex, treeSize, proof, leafHash)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, root) {
		return fmt.Errorf("got root:\n%x\nexpected:\n%x", got, root)
	}
	if err := v.VerifyInclusionProof(leafIndex, treeSize, proof, root, leafHash); err != nil {
		return err
	}

	probes := corruptInclVerification(leafIndex, treeSize, proof, root, leafHash)
	for _, p := range probes {
		if err := v.VerifyInclusionProof(p.leafIndex, p.treeSize, p.proof, p.root, p.leafHash); err == nil {
			return fmt.Errorf("incorrectly verified against: %s", p.desc)
		}
	}
	return nil
}

func verifierConsistencyCheck(v *LogVerifier, snapshot1, snapshot2 int64, root1, root2 []byte, proof [][]byte) error {
	// Verify original consistency proof.
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root1, root2, proof); err != nil {
		return err
	}
	// For simplicity test only non-trivial proofs that have root1 != root2,
	// snapshot1 != 0 and snapshot1 != snapshot2.
	if len(proof) == 0 {
		return nil
	}

	probes := corruptConsVerification(snapshot1, snapshot2, root1, root2, proof)
	for _, p := range probes {
		if err := v.VerifyConsistencyProof(p.snapshot1, p.snapshot2, p.root1, p.root2, p.proof); err == nil {
			return fmt.Errorf("incorrectly verified against: %s", p.desc)
		}
	}
	return nil
}

func TestVerifyInclusionProof(t *testing.T) {
	v := NewLogVerifier(rfc6962.DefaultHasher)
	path := [][]byte{}

	invalidPaths := []struct {
		index, size int64
	}{{0, 0}, {0, 1}, {1, 0}, {2, 1}}
	for i, p := range invalidPaths {
		if err := v.VerifyInclusionProof(p.index, p.size, path, []byte{}, []byte{1}); err == nil {
			t.Errorf("Incorrectly verified invalid path %d", i)
		}
		if err := v.VerifyInclusionProof(p.index, p.size, path, sha256EmptyTreeHash, []byte{}); err == nil {
			t.Errorf("Incorrectly verified invalid root %d", i)
		}
	}

	// i = 0 is an invalid path.
	for i := 1; i < 6; i++ {
		p := inclusionProofs[i]
		// Construct the path.
		proof := [][]byte{}
		for j := int64(0); j < p.proofLength; j++ {
			proof = append(proof, p.proof[j].h)
		}
		leafHash, err := rfc6962.DefaultHasher.HashLeaf(leaves[p.leaf-1].h)
		if err != nil {
			t.Fatalf("HashLeaf(): %v", err)
		}
		if err := verifierCheck(&v, p.leaf-1, p.snapshot, proof, roots[p.snapshot-1].h, leafHash); err != nil {
			t.Errorf("%d: verifierCheck(): %s", i, err)
		}
	}

}

func TestVerifyConsistencyProof(t *testing.T) {
	v := NewLogVerifier(rfc6962.DefaultHasher)

	proof := [][]byte{}
	root1 := []byte("don't care")
	root2 := []byte("don't care")

	// Snapshots that are always consistent
	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}
	if err := verifierConsistencyCheck(&v, 0, 1, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}

	// Invalid consistency proofs.
	// Time travel to the past.
	if err := verifierConsistencyCheck(&v, 1, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}
	if err := verifierConsistencyCheck(&v, 2, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}

	// Empty proof
	if err := verifierConsistencyCheck(&v, 1, 2, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}

	root1 = sha256EmptyTreeHash
	// Roots don't match.
	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified mismatched root")
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified mismatched root")
	}

	// Roots match but the proof is not empty.
	root2 = sha256EmptyTreeHash
	proof = [][]byte{sha256EmptyTreeHash}

	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}
	if err := verifierConsistencyCheck(&v, 0, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}

	for i := 0; i < 4; i++ {
		proof := [][]byte{}
		for j := int64(0); j < consistencyProofs[i].proofLen; j++ {
			proof = append(proof, consistencyProofs[i].proof[j].h)
		}
		snapshot1 := consistencyProofs[i].snapshot1
		snapshot2 := consistencyProofs[i].snapshot2
		err := verifierConsistencyCheck(&v, snapshot1, snapshot2,
			roots[snapshot1-1].h,
			roots[snapshot2-1].h, proof)
		if err != nil {
			t.Fatalf("Failed to verify known good proof for i=%d: %s", i, err)
		}
	}
}

func dh(h string) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return r
}

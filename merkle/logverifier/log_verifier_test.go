// Copyright 2017 Google LLC. All Rights Reserved.
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

package logverifier

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	_ "github.com/golang/glog"
	"github.com/google/trillian/merkle/rfc6962" // nolint:staticcheck
	inmemory "github.com/transparency-dev/merkle/testonly"
)

type inclusionProofTestVector struct {
	leaf     int64
	snapshot int64
	proof    [][]byte
}

type consistencyTestVector struct {
	snapshot1 int64
	snapshot2 int64
	proof     [][]byte
}

var (
	sha256SomeHash      = dh("abacaba000000000000000000000000000000000000000000060061e00123456", 32)
	sha256EmptyTreeHash = dh("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 32)

	inclusionProofs = []inclusionProofTestVector{
		{0, 0, nil},
		{1, 1, nil},
		{1, 8, [][]byte{
			dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7", 32),
			dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e", 32),
			dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4", 32),
		}},
		{6, 8, [][]byte{
			dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b", 32),
			dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0", 32),
			dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7", 32),
		}},
		{3, 3, [][]byte{
			dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125", 32),
		}},
		{2, 5, [][]byte{
			dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d", 32),
			dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e", 32),
			dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b", 32),
		}},
	}

	consistencyProofs = []consistencyTestVector{
		{1, 1, nil},
		{1, 8, [][]byte{
			dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7", 32),
			dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e", 32),
			dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4", 32),
		}},
		{6, 8, [][]byte{
			dh("0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a", 32),
			dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0", 32),
			dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7", 32),
		}},
		{2, 5, [][]byte{
			dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e", 32),
			dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b", 32),
		}},
	}

	roots = [][]byte{
		dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d", 32),
		dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125", 32),
		dh("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77", 32),
		dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7", 32),
		dh("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4", 32),
		dh("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef", 32),
		dh("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c", 32),
		dh("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328", 32),
	}

	leaves = [][]byte{
		dh("", 0),
		dh("00", 1),
		dh("10", 1),
		dh("2021", 2),
		dh("3031", 2),
		dh("40414243", 4),
		dh("5051525354555657", 8),
		dh("606162636465666768696a6b6c6d6e6f", 16),
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

func corruptInclusionProof(leafIndex, treeSize int64, proof [][]byte, root, leafHash []byte) []inclusionProbe {
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
		{leafIndex, treeSize, root, leafHash, extend(proof, []byte{}), "trailing garbage"},
		{leafIndex, treeSize, root, leafHash, extend(proof, root), "trailing root"},
		// Add garbage at the front.
		{leafIndex, treeSize, root, leafHash, prepend(proof, []byte{}), "preceding garbage"},
		{leafIndex, treeSize, root, leafHash, prepend(proof, root), "preceding root"},
	}
	ln := len(proof)

	// Modify single bit in an element of the proof.
	for i := 0; i < ln; i++ {
		wrongProof := prepend(proof)                          // Copy the proof slice.
		wrongProof[i] = append([]byte(nil), wrongProof[i]...) // But also the modified data.
		wrongProof[i][0] ^= 8                                 // Flip the bit.
		desc := fmt.Sprintf("modified proof[%d] bit 3", i)
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, wrongProof, desc})
	}

	if ln > 0 {
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, proof[:ln-1], "removed component"})
	}
	if ln > 1 {
		wrongProof := prepend(proof[1:], proof[0], sha256SomeHash)
		ret = append(ret, inclusionProbe{leafIndex, treeSize, root, leafHash, wrongProof, "inserted component"})
	}

	return ret
}

func corruptConsistencyProof(snapshot1, snapshot2 int64, root1, root2 []byte, proof [][]byte) []consistencyProbe {
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
		{snapshot1, snapshot2, root1, root2, extend(proof, []byte{}), "trailing garbage"},
		{snapshot1, snapshot2, root1, root2, extend(proof, root1), "trailing root1"},
		{snapshot1, snapshot2, root1, root2, extend(proof, root2), "trailing root2"},
		// Add garbage at the front.
		{snapshot1, snapshot2, root1, root2, prepend(proof, []byte{}), "preceding garbage"},
		{snapshot1, snapshot2, root1, root2, prepend(proof, root1), "preceding root1"},
		{snapshot1, snapshot2, root1, root2, prepend(proof, root2), "preceding root2"},
		{snapshot1, snapshot2, root1, root2, prepend(proof, proof[0]), "preceding proof[0]"},
	}

	// Remove a node from the end.
	if ln > 0 {
		ret = append(ret, consistencyProbe{snapshot1, snapshot2, root1, root2, proof[:ln-1], "truncated proof"})
	}

	// Modify single bit in an element of the proof.
	for i := 0; i < ln; i++ {
		wrongProof := prepend(proof)                          // Copy the proof slice.
		wrongProof[i] = append([]byte(nil), wrongProof[i]...) // But also the modified data.
		wrongProof[i][0] ^= 16                                // Flip the bit.
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

	probes := corruptInclusionProof(leafIndex, treeSize, proof, root, leafHash)
	var wrong []string
	for _, p := range probes {
		if err := v.VerifyInclusionProof(p.leafIndex, p.treeSize, p.proof, p.root, p.leafHash); err == nil {
			wrong = append(wrong, p.desc)
		}
	}
	if len(wrong) > 0 {
		return fmt.Errorf("incorrectly verified against: %s", strings.Join(wrong, ", "))
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

	probes := corruptConsistencyProof(snapshot1, snapshot2, root1, root2, proof)
	var wrong []string
	for _, p := range probes {
		if err := v.VerifyConsistencyProof(p.snapshot1, p.snapshot2, p.root1, p.root2, p.proof); err == nil {
			wrong = append(wrong, p.desc)
		}
	}
	if len(wrong) > 0 {
		return fmt.Errorf("incorrectly verified against: %s", strings.Join(wrong, ", "))
	}
	return nil
}

func TestVerifyInclusionProofSingleEntry(t *testing.T) {
	v := New(rfc6962.DefaultHasher)
	data := []byte("data")
	// Root and leaf hash for 1-entry tree are the same.
	hash := v.hasher.HashLeaf(data)
	// The corresponding inclusion proof is empty.
	proof := [][]byte{}
	emptyHash := []byte{}

	for i, tc := range []struct {
		root    []byte
		leaf    []byte
		wantErr bool
	}{
		{hash, hash, false},
		{hash, emptyHash, true},
		{emptyHash, hash, true},
		{emptyHash, emptyHash, true}, // Wrong hash size.
	} {
		t.Run(fmt.Sprintf("test:%d", i), func(t *testing.T) {
			err := v.VerifyInclusionProof(0, 1, proof, tc.root, tc.leaf)
			if got, want := err != nil, tc.wantErr; got != want {
				t.Errorf("error: %v, want %v", got, want)
			}
		})
	}
}

func TestVerifyInclusionProof(t *testing.T) {
	v := New(rfc6962.DefaultHasher)
	proof := [][]byte{}

	probes := []struct {
		index, size int64
	}{{0, 0}, {0, 1}, {1, 0}, {2, 1}}
	for _, p := range probes {
		t.Run(fmt.Sprintf("probe:%d:%d", p.index, p.size), func(t *testing.T) {
			if err := v.VerifyInclusionProof(p.index, p.size, proof, []byte{}, sha256SomeHash); err == nil {
				t.Error("Incorrectly verified invalid root/leaf")
			}
			if err := v.VerifyInclusionProof(p.index, p.size, proof, sha256EmptyTreeHash, []byte{}); err == nil {
				t.Error("Incorrectly verified invalid root/leaf")
			}
			if err := v.VerifyInclusionProof(p.index, p.size, proof, sha256EmptyTreeHash, sha256SomeHash); err == nil {
				t.Error("Incorrectly verified invalid root/leaf")
			}
		})
	}

	// i = 0 is an invalid path.
	for i := 1; i < 6; i++ {
		p := inclusionProofs[i]
		t.Run(fmt.Sprintf("proof:%d", i), func(t *testing.T) {
			leafHash := rfc6962.DefaultHasher.HashLeaf(leaves[p.leaf-1])
			if err := verifierCheck(&v, p.leaf-1, p.snapshot, p.proof, roots[p.snapshot-1], leafHash); err != nil {
				t.Errorf("verifierCheck(): %s", err)
			}
		})
	}
}

func TestVerifyInclusionProofGenerated(t *testing.T) {
	var sizes []int64
	for s := 1; s <= 70; s++ {
		sizes = append(sizes, int64(s))
	}
	sizes = append(sizes, []int64{1024, 5050}...)

	tree, v := createTree(0)
	for _, size := range sizes {
		growTree(tree, size)
		root := tree.Hash()
		for i := int64(0); i < size; i++ {
			t.Run(fmt.Sprintf("size:%d:index:%d", size, i), func(t *testing.T) {
				leaf, proof := getLeafAndProof(tree, i)
				if err := verifierCheck(&v, i, size, proof, root, leaf); err != nil {
					t.Errorf("verifierCheck(): %v", err)
				}
			})
		}
	}
}

func TestVerifyConsistencyProof(t *testing.T) {
	v := New(rfc6962.DefaultHasher)

	root1 := []byte("don't care 1")
	root2 := []byte("don't care 2")
	proof1 := [][]byte{}
	proof2 := [][]byte{sha256EmptyTreeHash}

	tests := []struct {
		snap1, snap2 int64
		root1, root2 []byte
		proof        [][]byte
		wantErr      bool
	}{
		{0, 0, root1, root2, proof1, true},
		{1, 1, root1, root2, proof1, true},
		// Snapshots that are always consistent.
		{0, 0, root1, root1, proof1, false},
		{0, 1, root1, root2, proof1, false},
		{1, 1, root2, root2, proof1, false},
		// Time travel to the past.
		{1, 0, root1, root2, proof1, true},
		{2, 1, root1, root2, proof1, true},
		// Empty proof.
		{1, 2, root1, root2, proof1, true},
		// Roots don't match.
		{0, 0, sha256EmptyTreeHash, root2, proof1, true},
		{1, 1, sha256EmptyTreeHash, root2, proof1, true},
		// Roots match but the proof is not empty.
		{0, 0, sha256EmptyTreeHash, sha256EmptyTreeHash, proof2, true},
		{0, 1, sha256EmptyTreeHash, sha256EmptyTreeHash, proof2, true},
		{1, 1, sha256EmptyTreeHash, sha256EmptyTreeHash, proof2, true},
	}
	for i, p := range tests {
		t.Run(fmt.Sprintf("test:%d:snap:%d-%d", i, p.snap1, p.snap2), func(t *testing.T) {
			err := verifierConsistencyCheck(&v, p.snap1, p.snap2, p.root1, p.root2, p.proof)
			if p.wantErr && err == nil {
				t.Errorf("Incorrectly verified")
			} else if !p.wantErr && err != nil {
				t.Errorf("Failed to verify: %v", err)
			}
		})
	}

	for i := 0; i < 4; i++ {
		p := consistencyProofs[i]
		t.Run(fmt.Sprintf("proof:%d", i), func(t *testing.T) {
			err := verifierConsistencyCheck(&v, p.snapshot1, p.snapshot2,
				roots[p.snapshot1-1], roots[p.snapshot2-1], p.proof)
			if err != nil {
				t.Fatalf("Failed to verify known good proof: %s", err)
			}
		})
	}
}

func TestVerifyConsistencyProofGenerated(t *testing.T) {
	size := int64(130)
	tree, v := createTree(size)
	roots := make([][]byte, size+1)
	for i := int64(0); i <= size; i++ {
		roots[i] = tree.HashAt(uint64(i))
	}

	for i := int64(0); i <= size; i++ {
		for j := i; j <= size; j++ {
			proof, err := tree.ConsistencyProof(uint64(i), uint64(j))
			if err != nil {
				t.Fatalf("ConsistencyProof: %v", err)
			}
			t.Run(fmt.Sprintf("size:%d:consistency:%d-%d", size, i, j), func(t *testing.T) {
				if err := verifierConsistencyCheck(&v, i, j, roots[i], roots[j], proof); err != nil {
					t.Errorf("verifierConsistencyCheck(): %v", err)
				}
			})
		}
	}
}

func TestPrefixHashFromInclusionProofGenerated(t *testing.T) {
	var sizes []int64
	for s := 1; s <= 258; s++ {
		sizes = append(sizes, int64(s))
	}
	sizes = append(sizes, []int64{1024, 5050, 10000}...)

	tree, v := createTree(0)
	for _, size := range sizes {
		growTree(tree, size)
		root := tree.Hash()

		for i := int64(1); i <= size; i++ {
			t.Run(fmt.Sprintf("size:%d:prefix:%d", size, i), func(t *testing.T) {
				leaf, proof := getLeafAndProof(tree, i-1)
				pRoot, err := v.VerifiedPrefixHashFromInclusionProof(i, size, proof, root, leaf)
				if err != nil {
					t.Fatalf("VerifiedPrefixHashFromInclusionProof(): %v", err)
				}
				exp := tree.HashAt(uint64(i))
				if !bytes.Equal(pRoot, exp) {
					t.Fatalf("wrong prefix hash: %s, want %s", shortHash(pRoot), shortHash(exp))
				}
			})
		}
	}
}

func TestPrefixHashFromInclusionProofErrors(t *testing.T) {
	size := int64(307)
	tree, v := createTree(size)
	root := tree.Hash()

	leaf2, proof2 := getLeafAndProof(tree, 2)
	_, proof3 := getLeafAndProof(tree, 3)
	_, proof301 := getLeafAndProof(tree, 301)

	idxTests := []struct {
		index int64
		size  int64
	}{
		{-1, -1},
		{-10, -1},
		{-1, -10},
		{10, -1},
		{10, 0},
		{10, 9},
		{0, 10},
		{0, -1},
		{0, 0},
		{-1, 0},
		{-1, size},
		{0, size},
		{size, size},
		{size + 1, size},
		{size + 100, size},
	}
	for _, it := range idxTests {
		if _, err := v.VerifiedPrefixHashFromInclusionProof(it.index, it.size, proof2, root, leaf2); err == nil {
			t.Errorf("VerifiedPrefixHashFromInclusionProof(%d,%d): expected error", it.index, it.size)
		}
	}

	if _, err := v.VerifiedPrefixHashFromInclusionProof(3, size, proof2, root, leaf2); err != nil {
		t.Errorf("VerifiedPrefixHashFromInclusionProof(): %v, expected no error", err)
	}

	// Proof #3 has the same length, but doesn't verify against index #2.
	// Neither does proof #301 as it has a different length.
	for _, proof := range [][][]byte{proof3, proof301} {
		if _, err := v.VerifiedPrefixHashFromInclusionProof(3, size, proof, root, leaf2); err == nil {
			t.Error("VerifiedPrefixHashFromInclusionProof(): expected error")
		}
	}
}

// extend explicitly copies |proof| slice and appends |hashes| to it.
func extend(proof [][]byte, hashes ...[]byte) [][]byte {
	res := make([][]byte, len(proof), len(proof)+len(hashes))
	copy(res, proof)
	return append(res, hashes...)
}

// prepend adds |proof| to the tail of |hashes|.
func prepend(proof [][]byte, hashes ...[]byte) [][]byte {
	return append(hashes, proof...)
}

func dh(h string, expLen int) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	if got := len(r); got != expLen {
		panic(fmt.Sprintf("decode %q: len=%d, want %d", h, got, expLen))
	}
	return r
}

func shortHash(hash []byte) string {
	if len(hash) == 0 {
		return "<empty>"
	}
	return fmt.Sprintf("%x...", hash[:4])
}

func createTree(size int64) (*inmemory.Tree, LogVerifier) {
	tree := inmemory.New(rfc6962.DefaultHasher)
	growTree(tree, size)
	return tree, New(rfc6962.DefaultHasher)
}

func growTree(tree *inmemory.Tree, upTo int64) {
	for i := int64(tree.Size()); i < upTo; i++ {
		tree.AppendData([]byte(fmt.Sprintf("data:%d", i)))
	}
}

func getLeafAndProof(tree *inmemory.Tree, index int64) ([]byte, [][]byte) {
	// Note: inmemory.MerkleTree counts leaves from 1.
	proof, err := tree.InclusionProof(uint64(index), tree.Size())
	if err != nil {
		panic(err)
	}
	leafHash := tree.LeafHash(uint64(index))
	return leafHash, proof
}

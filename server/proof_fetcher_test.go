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

package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
)

// rehashTest encapsulates one test case for the rehasher in isolation. Input data like the storage
// hashes and revisions can be arbitrary but the nodes should have distinct values
type rehashTest struct {
	desc    string
	index   int64
	nodes   []storage.Node
	fetches []merkle.NodeFetch
	output  *trillian.Proof
}

// An arbitrary tree revision to be used in tests.
const testTreeRevision int64 = 3

// Raw hashes for dummy storage nodes
var h1 = th.HashLeaf([]byte("Hash 1"))
var h2 = th.HashLeaf([]byte("Hash 2"))
var h3 = th.HashLeaf([]byte("Hash 3"))
var h4 = th.HashLeaf([]byte("Hash 4"))
var h5 = th.HashLeaf([]byte("Hash 5"))

// And the dummy nodes themselves.
var sn1 = storage.Node{NodeID: *storage.NewNodeIDFromHash(h1), Hash: h1, NodeRevision: 11}
var sn2 = storage.Node{NodeID: *storage.NewNodeIDFromHash(h2), Hash: h2, NodeRevision: 22}
var sn3 = storage.Node{NodeID: *storage.NewNodeIDFromHash(h3), Hash: h3, NodeRevision: 33}
var sn4 = storage.Node{NodeID: *storage.NewNodeIDFromHash(h4), Hash: h4, NodeRevision: 44}
var sn5 = storage.Node{NodeID: *storage.NewNodeIDFromHash(h5), Hash: h5, NodeRevision: 55}

func TestRehasher(t *testing.T) {
	hasher := rfc6962.DefaultHasher
	rehashTests := []rehashTest{
		{
			desc:    "no rehash",
			index:   126,
			nodes:   []storage.Node{sn1, sn2, sn3},
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: false}, {Rehash: false}},
			output: &trillian.Proof{
				LeafIndex: 126,
				Hashes:    [][]byte{h1, h2, h3},
			},
		},
		{
			desc:    "single rehash",
			index:   999,
			nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: false}},
			output: &trillian.Proof{
				LeafIndex: 999,
				Hashes:    [][]byte{h1, th.HashChildren(h3, h2), h4, h5},
			},
		},
		{
			desc:    "single rehash at end",
			index:   11,
			nodes:   []storage.Node{sn1, sn2, sn3},
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}},
			output: &trillian.Proof{
				LeafIndex: 11,
				Hashes:    [][]byte{h1, th.HashChildren(h3, h2)},
			},
		},
		{
			desc:    "single rehash multiple nodes",
			index:   23,
			nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: true}, {Rehash: false}},
			output: &trillian.Proof{
				LeafIndex: 23,
				Hashes:    [][]byte{h1, th.HashChildren(h4, th.HashChildren(h3, h2)), h5},
			},
		},
		{
			desc:    "multiple rehash",
			index:   45,
			nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
			fetches: []merkle.NodeFetch{{Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: true}, {Rehash: true}},
			output: &trillian.Proof{
				LeafIndex: 45,
				Hashes:    [][]byte{th.HashChildren(h2, h1), h3, th.HashChildren(h5, h4)},
			},
		},
	}

	for _, rehashTest := range rehashTests {
		r := &rehasher{th: hasher}
		for i, node := range rehashTest.nodes {
			r.process(node, rehashTest.fetches[i])
		}

		want := rehashTest.output
		got, err := r.rehashedProof(rehashTest.index)

		if err != nil {
			t.Fatalf("rehash test %s unexpected error: %v", rehashTest.desc, err)
		}

		if !proto.Equal(got, want) {
			t.Errorf("rehash test %s:\ngot: %v\nwant: %v", rehashTest.desc, got, want)
		}
	}
}

func TestTree813FetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher
	const ts int64 = 813

	mt := treeAtSize(int(ts))
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, int(ts-1)), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for l := int64(271); l < ts; l++ {
		fetches, err := merkle.CalcInclusionProofNodeAddresses(ts, l, ts)

		if err != nil {
			t.Fatal(err)
		}

		proof, err := fetchNodesAndBuildProof(ctx, r, hasher, testTreeRevision, int64(l), fetches)
		if err != nil {
			t.Fatal(err)
		}

		// We use +1 here because of the 1 based leaf indexing of this implementation
		refProof := mt.PathToRootAtSnapshot(l+1, ts)

		if got, want := len(proof.Hashes), len(refProof); got != want {
			for i, f := range fetches {
				t.Errorf("Fetch: %d => %+v", i, f.ID)
			}
			t.Fatalf("(%d, %d): got proof len: %d, want: %d: %v\n%v", ts, l, got, want, fetches, refProof)
		}

		for i := 0; i < len(proof.Hashes); i++ {
			if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
				t.Fatalf("(%d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", ts, l, i, got, want, len(proof.Hashes), fetches)
			}
		}
	}
}

func TestTree32InclusionProofFetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher
	for ts := 2; ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s := int64(2); s <= int64(ts); s++ {
			for l := int64(0); l < s; l++ {
				fetches, err := merkle.CalcInclusionProofNodeAddresses(s, l, int64(ts))
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, testTreeRevision, int64(l), fetches)
				if err != nil {
					t.Fatal(err)
				}

				// We use +1 here because of the 1 based leaf indexing of this implementation
				refProof := mt.PathToRootAtSnapshot(l+1, s)

				if got, want := len(proof.Hashes), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s, l, got, want, fetches, refProof)
				}

				for i := 0; i < len(proof.Hashes); i++ {
					if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", ts, s, l, i, got, want, len(proof.Hashes), fetches)
					}
				}
			}
		}
	}
}

func TestTree32InclusionProofFetchMultiBatch(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher

	mt := treeAtSize(32)
	// The reader is built up with multiple batches, 4 batches x 8 leaves each
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, 7), ExpectedRoot: expectedRootAtSize(treeAtSize(8))},
		{TreeRevision: testTreeRevision + 1, Leaves: expandLeaves(8, 15), ExpectedRoot: expectedRootAtSize(treeAtSize(16))},
		{TreeRevision: testTreeRevision + 2, Leaves: expandLeaves(16, 23), ExpectedRoot: expectedRootAtSize(treeAtSize(24))},
		{TreeRevision: testTreeRevision + 3, Leaves: expandLeaves(24, 31), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for s := int64(2); s <= 32; s++ {
		for l := int64(0); l < s; l++ {
			fetches, err := merkle.CalcInclusionProofNodeAddresses(s, l, 32)
			if err != nil {
				t.Fatal(err)
			}

			// Use the highest tree revision that should be available from the node reader
			proof, err := fetchNodesAndBuildProof(ctx, r, hasher, testTreeRevision+3, l, fetches)
			if err != nil {
				t.Fatal(err)
			}

			// We use +1 here because of the 1 based leaf indexing of this implementation
			refProof := mt.PathToRootAtSnapshot(l+1, s)

			if got, want := len(proof.Hashes), len(refProof); got != want {
				t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", 32, s, l, got, want, fetches, refProof)
			}

			for i := 0; i < len(proof.Hashes); i++ {
				if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
					t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", 32, s, l, i, got, want, len(proof.Hashes), fetches)
				}
			}
		}
	}
}

func TestTree32ConsistencyProofFetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher
	for ts := 2; ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s1 := int64(2); s1 < int64(ts); s1++ {
			for s2 := int64(s1 + 1); s2 < int64(ts); s2++ {
				fetches, err := merkle.CalcConsistencyProofNodeAddresses(s1, s2, int64(ts))
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, testTreeRevision, int64(s1), fetches)
				if err != nil {
					t.Fatal(err)
				}

				refProof := mt.SnapshotConsistency(s1, s2)

				if got, want := len(proof.Hashes), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s1, s2, got, want, fetches, refProof)
				}

				for i := 0; i < len(proof.Hashes); i++ {
					if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", ts, s1, s2, i, got, want, len(proof.Hashes), fetches)
					}
				}
			}
		}
	}
}

func expandLeaves(n, m int) []string {
	leaves := make([]string, 0, m-n+1)
	for l := n; l <= m; l++ {
		leaves = append(leaves, fmt.Sprintf("Leaf %d", l))
	}
	return leaves
}

// expectedRootAtSize uses the in memory tree, the tree built with Compact Merkle Tree should
// have the same root.
func expectedRootAtSize(mt *merkle.InMemoryMerkleTree) []byte {
	return mt.CurrentRoot().Hash()
}

func treeAtSize(n int) *merkle.InMemoryMerkleTree {
	leaves := expandLeaves(0, n-1)
	mt := merkle.NewInMemoryMerkleTree(rfc6962.DefaultHasher)
	for _, leaf := range leaves {
		mt.AddLeaf([]byte(leaf))
	}
	return mt
}

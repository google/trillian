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

package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
)

// An arbitrary tree revision to be used in tests.
const testTreeRevision int64 = 3

func TestRehasher(t *testing.T) {
	th := rfc6962.DefaultHasher
	h := [][]byte{
		th.HashLeaf([]byte("Hash 1")),
		th.HashLeaf([]byte("Hash 2")),
		th.HashLeaf([]byte("Hash 3")),
		th.HashLeaf([]byte("Hash 4")),
		th.HashLeaf([]byte("Hash 5")),
	}
	// Note: Node IDs and revisions are ignored by the algorithm.
	nodes := []tree.Node{
		{Hash: h[0]}, {Hash: h[1]}, {Hash: h[2]}, {Hash: h[3]}, {Hash: h[4]}}

	for _, tc := range []struct {
		desc    string
		index   int64
		nodes   []tree.Node
		fetches []merkle.NodeFetch
		want    [][]byte
	}{
		{
			desc:    "no rehash",
			index:   126,
			nodes:   nodes[:3],
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: false}, {Rehash: false}},
			want:    h[:3],
		},
		{
			desc:    "single rehash",
			index:   999,
			nodes:   nodes[:5],
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: false}},
			want:    [][]byte{h[0], th.HashChildren(h[2], h[1]), h[3], h[4]},
		},
		{
			desc:    "single rehash at end",
			index:   11,
			nodes:   nodes[:3],
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}},
			want:    [][]byte{h[0], th.HashChildren(h[2], h[1])},
		},
		{
			desc:    "single rehash multiple nodes",
			index:   23,
			nodes:   nodes[:5],
			fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: true}, {Rehash: false}},
			want:    [][]byte{h[0], th.HashChildren(h[3], th.HashChildren(h[2], h[1])), h[4]},
		},
		{
			desc:    "multiple rehash",
			index:   45,
			nodes:   nodes[:5],
			fetches: []merkle.NodeFetch{{Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: true}, {Rehash: true}},
			want:    [][]byte{th.HashChildren(h[1], h[0]), h[2], th.HashChildren(h[4], h[3])},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			r := &rehasher{th: th}
			for i, node := range tc.nodes {
				r.process(node, tc.fetches[i])
			}

			want := &trillian.Proof{
				LeafIndex: tc.index,
				Hashes:    tc.want,
			}
			got, err := r.rehashedProof(tc.index)

			if err != nil {
				t.Fatalf("rehashedProof: %v", err)
			}

			if !proto.Equal(got, want) {
				t.Errorf("proofs mismatch:\ngot: %v\nwant: %v", got, want)
			}
		})
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

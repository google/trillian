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

	"github.com/google/trillian/internal/merkle/inmemory"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage/testonly"
)

// An arbitrary tree revision to be used in tests.
const testTreeRevision int64 = 3

func TestTree813FetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher
	const ts int64 = 813

	mt := treeAtSize(int(ts))
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, int(ts-1)), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for l := int64(271); l < ts; l++ {
		pn, err := merkle.CalcInclusionProofNodeAddresses(ts, l)
		if err != nil {
			t.Fatal(err)
		}

		proof, err := fetchNodesAndBuildProof(ctx, r, hasher, int64(l), pn)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := proof.LeafIndex, int64(l); got != want {
			t.Errorf("leaf index mismatch: got %d, want %d", got, want)
		}

		// We use +1 here because of the 1 based leaf indexing of this implementation
		refProof := mt.PathToRootAtSnapshot(l+1, ts)

		if got, want := len(proof.Hashes), len(refProof); got != want {
			for i, id := range pn.IDs {
				t.Errorf("Fetch: %d => %+v", i, id)
			}
			t.Fatalf("(%d, %d): got proof len: %d, want: %d: %v\n%v", ts, l, got, want, pn, refProof)
		}

		for i := 0; i < len(proof.Hashes); i++ {
			if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
				t.Fatalf("(%d, %d): %d got proof node: %s, want: %s l:%d nodes: %v", ts, l, i, got, want, len(proof.Hashes), pn)
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
				fetches, err := merkle.CalcInclusionProofNodeAddresses(s, l)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, int64(l), fetches)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := proof.LeafIndex, int64(l); got != want {
					t.Errorf("leaf index mismatch: got %d, want %d", got, want)
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
			fetches, err := merkle.CalcInclusionProofNodeAddresses(s, l)
			if err != nil {
				t.Fatal(err)
			}

			// Use the highest tree revision that should be available from the node reader
			proof, err := fetchNodesAndBuildProof(ctx, r, hasher, l, fetches)
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
				fetches, err := merkle.CalcConsistencyProofNodeAddresses(s1, s2)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, int64(s1), fetches)
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
func expectedRootAtSize(mt *inmemory.MerkleTree) []byte {
	return mt.CurrentRoot().Hash()
}

func treeAtSize(n int) *inmemory.MerkleTree {
	leaves := expandLeaves(0, n-1)
	mt := inmemory.NewMerkleTree(rfc6962.DefaultHasher)
	for _, leaf := range leaves {
		mt.AddLeaf([]byte(leaf))
	}
	return mt
}

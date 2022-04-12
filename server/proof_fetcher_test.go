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
	"github.com/google/trillian/storage/testonly"
	"github.com/transparency-dev/merkle/proof"
	"github.com/transparency-dev/merkle/rfc6962"
)

// An arbitrary tree revision to be used in tests.
const testTreeRevision int64 = 3

func TestTree813FetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher.HashChildren
	const ts uint64 = 813

	mt := treeAtSize(ts)
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for l := uint64(271); l < ts; l++ {
		nodes, err := proof.Inclusion(l, ts)
		if err != nil {
			t.Fatal(err)
		}

		proof, err := fetchNodesAndBuildProof(ctx, r, hasher, l, nodes)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := proof.LeafIndex, int64(l); got != want {
			t.Errorf("leaf index mismatch: got %d, want %d", got, want)
		}

		// We use +1 here because of the 1 based leaf indexing of this implementation
		refProof := mt.PathToRootAtSnapshot(int64(l+1), int64(ts))

		if got, want := len(proof.Hashes), len(refProof); got != want {
			for i, id := range nodes.IDs {
				t.Errorf("Fetch: %d => %+v", i, id)
			}
			t.Fatalf("(%d, %d): got proof len: %d, want: %d: %v\n%v", ts, l, got, want, nodes, refProof)
		}

		for i := 0; i < len(proof.Hashes); i++ {
			if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i]); got != want {
				t.Fatalf("(%d, %d): %d got proof node: %s, want: %s l:%d nodes: %v", ts, l, i, got, want, len(proof.Hashes), nodes)
			}
		}
	}
}

func TestTree32InclusionProofFetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher.HashChildren
	for ts := uint64(2); ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s := uint64(2); s <= ts; s++ {
			for l := uint64(0); l < s; l++ {
				nodes, err := proof.Inclusion(l, s)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, l, nodes)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := proof.LeafIndex, int64(l); got != want {
					t.Errorf("leaf index mismatch: got %d, want %d", got, want)
				}

				// We use +1 here because of the 1 based leaf indexing of this implementation
				refProof := mt.PathToRootAtSnapshot(int64(l+1), int64(s))

				if got, want := len(proof.Hashes), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s, l, got, want, nodes, refProof)
				}

				for i := 0; i < len(proof.Hashes); i++ {
					if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i]); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d nodes: %v", ts, s, l, i, got, want, len(proof.Hashes), nodes)
					}
				}
			}
		}
	}
}

func TestTree32InclusionProofFetchMultiBatch(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher.HashChildren

	mt := treeAtSize(32)
	// The reader is built up with multiple batches, 4 batches x 8 leaves each
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, 7), ExpectedRoot: expectedRootAtSize(treeAtSize(8))},
		{TreeRevision: testTreeRevision + 1, Leaves: expandLeaves(8, 15), ExpectedRoot: expectedRootAtSize(treeAtSize(16))},
		{TreeRevision: testTreeRevision + 2, Leaves: expandLeaves(16, 23), ExpectedRoot: expectedRootAtSize(treeAtSize(24))},
		{TreeRevision: testTreeRevision + 3, Leaves: expandLeaves(24, 31), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for s := uint64(2); s <= 32; s++ {
		for l := uint64(0); l < s; l++ {
			nodes, err := proof.Inclusion(l, s)
			if err != nil {
				t.Fatal(err)
			}

			// Use the highest tree revision that should be available from the node reader
			proof, err := fetchNodesAndBuildProof(ctx, r, hasher, l, nodes)
			if err != nil {
				t.Fatal(err)
			}

			// We use +1 here because of the 1 based leaf indexing of this implementation
			refProof := mt.PathToRootAtSnapshot(int64(l+1), int64(s))

			if got, want := len(proof.Hashes), len(refProof); got != want {
				t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", 32, s, l, got, want, nodes, refProof)
			}

			for i := 0; i < len(proof.Hashes); i++ {
				if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i]); got != want {
					t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d nodes: %v", 32, s, l, i, got, want, len(proof.Hashes), nodes)
				}
			}
		}
	}
}

func TestTree32ConsistencyProofFetchAll(t *testing.T) {
	ctx := context.Background()
	hasher := rfc6962.DefaultHasher.HashChildren
	for ts := uint64(2); ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: testTreeRevision, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s1 := uint64(2); s1 < ts; s1++ {
			for s2 := uint64(s1 + 1); s2 < ts; s2++ {
				nodes, err := proof.Consistency(s1, s2)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(ctx, r, hasher, s1, nodes)
				if err != nil {
					t.Fatal(err)
				}

				refProof := mt.SnapshotConsistency(int64(s1), int64(s2))

				if got, want := len(proof.Hashes), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s1, s2, got, want, nodes, refProof)
				}

				for i := 0; i < len(proof.Hashes); i++ {
					if got, want := hex.EncodeToString(proof.Hashes[i]), hex.EncodeToString(refProof[i]); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d nodes: %v", ts, s1, s2, i, got, want, len(proof.Hashes), nodes)
					}
				}
			}
		}
	}
}

func expandLeaves(n, m uint64) []string {
	leaves := make([]string, 0, m-n+1)
	for l := n; l <= m; l++ {
		leaves = append(leaves, fmt.Sprintf("Leaf %d", l))
	}
	return leaves
}

// expectedRootAtSize uses the in memory tree, the tree built with Compact Merkle Tree should
// have the same root.
func expectedRootAtSize(mt *inmemory.MerkleTree) []byte {
	return mt.CurrentRoot()
}

func treeAtSize(n uint64) *inmemory.MerkleTree {
	leaves := expandLeaves(0, n-1)
	mt := inmemory.NewMerkleTree(rfc6962.DefaultHasher)
	for _, leaf := range leaves {
		mt.AddLeaf([]byte(leaf))
	}
	return mt
}

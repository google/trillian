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

package maptest

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"

	tcrypto "github.com/google/trillian/crypto"
	stestonly "github.com/google/trillian/storage/testonly"

	_ "github.com/google/trillian/merkle/coniks"
	_ "github.com/google/trillian/merkle/maphasher"
)

var h2b = testonly.MustHexDecode

// createBatchLeaves produces n unique map leaves.
func createBatchLeaves(batch, n int) []*trillian.MapLeaf {
	leaves := make([]*trillian.MapLeaf, 0, n)
	for i := 0; i < n; i++ {
		leaves = append(leaves, &trillian.MapLeaf{
			Index:     testonly.TransparentHash(fmt.Sprintf("batch-%d-key-%d", batch, i)),
			LeafValue: []byte(fmt.Sprintf("batch-%d-value-%d", batch, i)),
		})
	}
	return leaves
}

func isEmptyMap(ctx context.Context, env *integration.MapEnv, tree *trillian.Tree) error {
	r, err := env.MapClient.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
		MapId: tree.TreeId,
	})
	if err != nil {
		return fmt.Errorf("failed to get empty map head: %v", err)
	}

	if got, want := r.GetMapRoot().GetMapRevision(), int64(0); got != want {
		return fmt.Errorf("got SMH with revision %d, want %d", got, want)
	}
	return nil
}

func verifyGetMapLeavesResponse(getResp *trillian.GetMapLeavesResponse, indexes [][]byte,
	wantRevision int64, pubKey crypto.PublicKey, hasher hashers.MapHasher, treeID int64) error {
	if got, want := len(getResp.MapLeafInclusion), len(indexes); got != want {
		return fmt.Errorf("got %d values, want %d", got, want)
	}
	if got, want := getResp.GetMapRoot().GetMapRevision(), wantRevision; got != want {
		return fmt.Errorf("got SMH with revision %d, want %d", got, want)
	}

	// SignedMapRoot contains its own signature. To verify, we need to create a local
	// copy of the object and return the object to the state it was in when signed
	// by removing the signature from the object.
	smr := *getResp.GetMapRoot()
	smr.Signature = nil // Remove the signature from the object to be verified.
	if err := tcrypto.VerifyObject(pubKey, smr, getResp.GetMapRoot().GetSignature()); err != nil {
		return fmt.Errorf("VerifyObject(SMR): %v", err)
	}
	rootHash := getResp.GetMapRoot().GetRootHash()
	for _, incl := range getResp.MapLeafInclusion {
		leaf := incl.GetLeaf().GetLeafValue()
		index := incl.GetLeaf().GetIndex()
		leafHash := incl.GetLeaf().GetLeafHash()
		proof := incl.GetInclusion()

		if got, want := leafHash, hasher.HashLeaf(treeID, index, leaf); !bytes.Equal(got, want) {
			return fmt.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
		}
		if err := merkle.VerifyMapInclusionProof(treeID, index,
			leafHash, rootHash, proof, hasher); err != nil {
			return fmt.Errorf("VerifyMapInclusionProof(%x): %v", index, err)
		}
	}
	return nil
}

// newTreeWithHasher is a test setup helper for creating new trees with a given hasher.
func newTreeWithHasher(ctx context.Context, env *integration.MapEnv, hashStrategy trillian.HashStrategy) (*trillian.Tree, hashers.MapHasher, error) {
	treeParams := stestonly.MapTree
	treeParams.HashStrategy = hashStrategy
	tree, err := env.AdminClient.CreateTree(ctx, &trillian.CreateTreeRequest{
		Tree: treeParams,
	})
	if err != nil {
		return nil, nil, nil
	}

	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, nil
	}
	return tree, hasher, nil
}

func TestLeafHistory(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc         string
		HashStrategy trillian.HashStrategy
		batches      [][]*trillian.MapLeaf
		get          []struct {
			revision  int64
			Index     []byte
			LeafValue []byte
		}
	}{
		{
			desc:         "single leaf update",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batches: [][]*trillian.MapLeaf{
				[]*trillian.MapLeaf{}, // Advance revision without changing anything.
				[]*trillian.MapLeaf{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				},
				[]*trillian.MapLeaf{}, // Advance revision without changing anything.
				[]*trillian.MapLeaf{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("B")},
				},
				[]*trillian.MapLeaf{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("C")},
				},
			},
			get: []struct {
				revision  int64
				Index     []byte
				LeafValue []byte
			}{
				{revision: 1, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: nil},
				{revision: 2, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{revision: 3, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{revision: 4, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("B")},
				{revision: 5, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("C")},
			},
		},
	} {
		tree, hasher, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
			continue
		}
		pubKey, err := keys.NewFromPublicDER(tree.GetPublicKey().GetDer())
		if err != nil {
			t.Errorf("%v: NewFromPublicDER(%v): %v", tc.desc, tc.HashStrategy, err)
			continue
		}

		for _, batch := range tc.batches {
			setResp, err := env.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
				MapId:  tree.TreeId,
				Leaves: batch,
			})
			if err != nil {
				t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
				continue
			}
			t.Logf("Rev: %v Set(): %x", setResp.GetMapRoot().GetMapRevision(), setResp.GetMapRoot().GetRootHash())
		}

		for _, batch := range tc.get {
			indexes := [][]byte{batch.Index}
			getResp, err := env.MapClient.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
				MapId:    tree.TreeId,
				Index:    indexes,
				Revision: batch.revision,
			})
			if err != nil {
				t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
				continue
			}
			t.Logf("Rev: %v Get(): %x", getResp.GetMapRoot().GetMapRevision(), getResp.GetMapRoot().GetRootHash())

			if got, want := getResp.MapLeafInclusion[0].GetLeaf().GetLeafValue(), batch.LeafValue; !bytes.Equal(got, want) {
				t.Errorf("GetLeaves(rev: %v).LeafValue: %s, want %s", batch.revision, got, want)
				continue
			}

			if err := verifyGetMapLeavesResponse(getResp, indexes, batch.revision,
				pubKey, hasher, tree.TreeId); err != nil {
				t.Errorf("%v: verifyGetMapLeavesResponse(rev %v): %v", tc.desc, batch.revision, err)
				continue
			}
		}
	}
}

func TestInclusion(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc         string
		HashStrategy trillian.HashStrategy
		leaves       []*trillian.MapLeaf
	}{
		{
			desc:         "maphasher single",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
			},
		},
		{
			desc:         "maphasher multi",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000001"), LeafValue: []byte("B")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000002"), LeafValue: []byte("C")},
			},
		},
		{
			desc:         "CONIKS across subtrees",
			HashStrategy: trillian.HashStrategy_CONIKS_SHA512_256,
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000180000000000000000000000000000000000000000000000000"), LeafValue: []byte("Z")},
			},
		},
		{
			desc:         "CONIKS multi",
			HashStrategy: trillian.HashStrategy_CONIKS_SHA512_256,
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000001"), LeafValue: []byte("B")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000002"), LeafValue: []byte("C")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000003"), LeafValue: nil},
			},
		},
	} {
		tree, hasher, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
			continue
		}
		pubKey, err := keys.NewFromPublicDER(tree.GetPublicKey().GetDer())
		if err != nil {
			t.Errorf("%v: NewFromPublicDER(%v): %v", tc.desc, tc.HashStrategy, err)
			continue
		}

		if _, err := env.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: tc.leaves,
			MapperData: &trillian.MapperMetadata{
				HighestFullyCompletedSeq: 0xcafe,
			},
		}); err != nil {
			t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
			continue
		}

		indexes := [][]byte{}
		for _, l := range tc.leaves {
			indexes = append(indexes, l.Index)
		}
		getResp, err := env.MapClient.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
			MapId:    tree.TreeId,
			Index:    indexes,
			Revision: -1,
		})
		if err != nil {
			t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
			continue
		}

		if err := verifyGetMapLeavesResponse(getResp, indexes, 1,
			pubKey, hasher, tree.TreeId); err != nil {
			t.Errorf("%v: verifyGetMapLeavesResponse(): %v", tc.desc, err)
			continue
		}
	}
}

func TestInclusionBatch(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc                  string
		HashStrategy          trillian.HashStrategy
		batchSize, numBatches int
		large                 bool
	}{

		{
			desc:         "maphasher short batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    10, numBatches: 10,
			large: false,
		},
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    64, numBatches: 32,
			large: true,
		},
		// TODO(gdbelvin): investigate batches of size > 150.
		// We are currently getting DB connection starvation: Too many connections.
	} {
		if testing.Short() && tc.large {
			t.Logf("testing.Short() is true. Skipping %v", tc.desc)
			continue
		}
		tree, _, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}

		if err := RunMapBatchTest(ctx, env, tree, tc.batchSize, tc.numBatches); err != nil {
			t.Errorf("BatchSize: %v, Batches: %v: %v", tc.batchSize, tc.numBatches, err)
		}
	}
}

// RunMapBatchTest runs a map integration test using the given tree and client.
func RunMapBatchTest(ctx context.Context, env *integration.MapEnv, tree *trillian.Tree,
	batchSize, numBatches int) error {
	// Parse variables from tree
	pubKey, err := keys.NewFromPublicDER(tree.GetPublicKey().GetDer())
	if err != nil {
		return err
	}
	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return err
	}

	// Ensure we're starting with an empty map
	if err := isEmptyMap(ctx, env, tree); err != nil {
		return err
	}

	// Generate leaves.
	leafBatch := make([][]*trillian.MapLeaf, numBatches)
	leafMap := make(map[string]*trillian.MapLeaf)
	for i := range leafBatch {
		leafBatch[i] = createBatchLeaves(i, batchSize)
		for _, l := range leafBatch[i] {
			leafMap[hex.EncodeToString(l.Index)] = l
		}
	}

	// Write some data in batches
	for _, b := range leafBatch {
		if _, err := env.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: b,
		}); err != nil {
			return fmt.Errorf("SetLeaves(): %v", err)
		}
	}

	// Check your head
	r, err := env.MapClient.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
		MapId: tree.TreeId,
	})
	if err != nil {
		return fmt.Errorf("failed to get map head: %v", err)
	}

	if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
		return fmt.Errorf("got SMH with revision %d, want %d", got, want)
	}

	// Shuffle the indexes. Map access is randomized.
	indexBatch := make([][][]byte, 0, numBatches)
	i := 0
	for _, v := range leafMap {
		if i%batchSize == 0 {
			indexBatch = append(indexBatch, make([][]byte, 0, batchSize))
		}
		batchIndex := i / batchSize
		indexBatch[batchIndex] = append(indexBatch[batchIndex], v.Index)
		i++
	}

	for i, indexes := range indexBatch {
		getResp, err := env.MapClient.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
			MapId:    tree.TreeId,
			Index:    indexes,
			Revision: -1,
		})
		if err != nil {
			return fmt.Errorf("GetLeaves(): %v", err)
		}

		if err := verifyGetMapLeavesResponse(getResp, indexes, int64(numBatches),
			pubKey, hasher, tree.TreeId); err != nil {
			return fmt.Errorf("batch %v: verifyGetMapLeavesResponse(): %v", i, err)
		}

		// Verify leaf contents
		for _, incl := range getResp.MapLeafInclusion {
			index := incl.GetLeaf().GetIndex()
			leaf := incl.GetLeaf().GetLeafValue()
			ev, ok := leafMap[hex.EncodeToString(index)]
			if !ok {
				return fmt.Errorf("unexpected key returned: %s", index)
			}
			if got, want := leaf, ev.LeafValue; !bytes.Equal(got, want) {
				return fmt.Errorf("got value %s, want %s", got, want)
			}
		}
	}
	return nil
}

func TestNonExistentLeaf(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc         string
		HashStrategy trillian.HashStrategy
		leaves       []*trillian.MapLeaf
		getIndex     []byte
	}{
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			leaves: []*trillian.MapLeaf{
				{Index: testonly.TransparentHash("A"), LeafValue: []byte("A")},
			},
			getIndex: []byte("doesnotexist...................."),
		},
	} {
		tree, hasher, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
			continue
		}

		if _, err := env.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: tc.leaves,
		}); err != nil {
			t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
			continue
		}

		r, err := env.MapClient.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
			MapId:    tree.TreeId,
			Revision: -1,
			Index:    [][]byte{tc.getIndex},
		})
		if err != nil {
			t.Errorf("GetLeaves(): %v", err)
			continue
		}

		rootHash := r.GetMapRoot().GetRootHash()
		for _, incl := range r.MapLeafInclusion {
			leaf := incl.GetLeaf().GetLeafValue()
			index := incl.GetLeaf().GetIndex()
			leafHash := incl.GetLeaf().GetLeafHash()
			proof := incl.GetInclusion()

			if got, want := len(leaf), 0; got != want {
				t.Errorf("len(leaf): %v, want, %v", got, want)
			}

			if got, want := leafHash, hasher.HashLeaf(tree.TreeId, index, leaf); !bytes.Equal(got, want) {
				t.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
			}
			if err := merkle.VerifyMapInclusionProof(tree.TreeId, index,
				leafHash, rootHash, proof, hasher); err != nil {
				t.Errorf("%v: VerifyMapInclusionProof(%x): %v", tc.desc, index, err)
			}
		}
	}
}

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

package integration

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"

	stestonly "github.com/google/trillian/storage/testonly"

	_ "github.com/google/trillian/merkle/coniks"
	_ "github.com/google/trillian/merkle/maphasher"
)

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

func TestInclusion(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, "TestInclusion")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	for _, tc := range []struct {
		desc string
		trillian.HashStrategy
		iv [][]byte
	}{
		{
			desc:         "maphasher single",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			iv: [][]byte{
				testonly.TransparentHash("A"), []byte("A"),
			},
		},
		{
			desc:         "maphasher multi",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			iv: [][]byte{
				testonly.TransparentHash("A"), []byte("A"),
				testonly.TransparentHash("B"), []byte("B"),
				testonly.TransparentHash("C"), []byte("C"),
			},
		},
	} {
		tree, hasher, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}
		mapID := tree.TreeId
		client := env.MapClient
		leaves := createMapLeaves(tc.iv...)

		if _, err := client.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  mapID,
			Leaves: leaves,
		}); err != nil {
			t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
			continue
		}

		indexes := [][]byte{}
		for _, l := range leaves {
			indexes = append(indexes, l.Index)
		}
		getResp, err := client.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
			MapId:    mapID,
			Index:    indexes,
			Revision: -1,
		})
		if err != nil {
			t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
			continue
		}

		rootHash := getResp.GetMapRoot().GetRootHash()
		for _, m := range getResp.GetMapLeafInclusion() {
			index := m.GetLeaf().GetIndex()
			leafHash := m.GetLeaf().GetLeafHash()
			proof := m.GetInclusion()
			if err := merkle.VerifyMapInclusionProof(mapID, index,
				leafHash, rootHash, proof, hasher); err != nil {
				t.Errorf("%v: VerifyMapInclusionProof(%x): %v", tc.desc, index, err)
			}
		}
	}
}

func TestInclusionBatch(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, "TestInclusionBatch")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
TestCase:
	for _, tc := range []struct {
		desc string
		trillian.HashStrategy
		batchSize, numBatches int
	}{
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    64, numBatches: 32,
		},
	} {
		tree, hasher, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}
		mapID := tree.TreeId
		client := env.MapClient

		leafBatch := make([][]*trillian.MapLeaf, tc.numBatches)
		leafMap := make(map[string]*trillian.MapLeaf)
		for i := range leafBatch {
			leafBatch[i] = createMapLeaves(makeBatchIndexValues(i, tc.batchSize)...)
			for _, l := range leafBatch[i] {
				leafMap[hex.EncodeToString(l.Index)] = l
			}
		}

		for _, b := range leafBatch {
			if _, err := client.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
				MapId:  mapID,
				Leaves: b,
			}); err != nil {
				t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
				continue TestCase
			}
		}

		// Shuffle the indexes. Map access is randomized.
		indexBatch := make([][][]byte, 0, tc.numBatches)
		i := 0
		for _, v := range leafMap {
			if i%tc.batchSize == 0 {
				indexBatch = append(indexBatch, make([][]byte, tc.batchSize))
			}
			indexBatch[i/tc.batchSize][i%tc.batchSize] = v.Index
			i++
		}
		for _, indexes := range indexBatch {
			getResp, err := client.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
				MapId:    mapID,
				Index:    indexes,
				Revision: -1,
			})
			if err != nil {
				t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
				continue TestCase
			}

			rootHash := getResp.GetMapRoot().GetRootHash()
			for _, m := range getResp.GetMapLeafInclusion() {
				index := m.GetLeaf().GetIndex()
				leafHash := m.GetLeaf().GetLeafHash()
				proof := m.GetInclusion()
				if err := merkle.VerifyMapInclusionProof(mapID, index,
					leafHash, rootHash, proof, hasher); err != nil {
					t.Errorf("%v: VerifyMapInclusionProof(): %v", tc.desc, err)
				}
			}
		}
	}
}

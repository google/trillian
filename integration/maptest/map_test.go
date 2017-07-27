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
	"fmt"
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

// createHashKV returns a []*trillian.MapLeaf formed by the mapping of index, value ...
// createHashKV panics if len(iv) is odd. Duplicate i/v pairs get over written.
func createMapLeaves(iv ...[]byte) []*trillian.MapLeaf {
	if len(iv)%2 != 0 {
		panic(fmt.Sprintf("integration: createMapLeaves got odd number of iv pairs: %v", len(iv)))
	}
	r := []*trillian.MapLeaf{}
	for i := 0; i < len(iv); i += 2 {
		r = append(r, &trillian.MapLeaf{
			Index:     iv[i],
			LeafValue: iv[i+1],
		})
	}
	return r
}

func TestInclusionWithEnv(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, "TestInclusionWithEnv")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	for _, tc := range []struct {
		desc string
		trillian.HashStrategy
		index, value []byte
	}{
		{
			desc:         "maphasher",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			index:        testonly.TransparentHash("A"),
			value:        []byte("A"),
		},
	} {
		treeParams := stestonly.MapTree
		treeParams.HashStrategy = tc.HashStrategy
		tree, err := env.AdminClient.CreateTree(ctx, &trillian.CreateTreeRequest{
			Tree: treeParams,
		})
		if err != nil {
			t.Errorf("%v: CreateTree(): %v", tc.desc, err)
			continue
		}

		mapID := tree.TreeId
		hasher, err := hashers.NewMapHasher(tree.HashStrategy)
		if err != nil {
			t.Errorf("%v: NewMapHasher(): %v", tc.desc, err)
			continue
		}
		client := env.MapClient
		leaves := createMapLeaves(tc.index, tc.value)

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
				t.Errorf("%v: VerifyMapInclusionProof(): %v", tc.desc, err)
			}
		}
	}
}

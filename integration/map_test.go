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
	"fmt"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	_ "github.com/google/trillian/merkle/coniks" // Register
	"github.com/google/trillian/merkle/hashers"
	_ "github.com/google/trillian/merkle/maphasher" // Register
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

// createHashKV returns a []*trillian.MapLeaf formed by the mapping of index, value ...
// createHashKV panics if len(iv) is odd. Duplicate i/v pairs get over written.
func createMapLeaves(iv ...[]byte) []*trillian.MapLeaf {
	if len(iv)%2 != 0 {
		panic(fmt.Sprintf("integration: createMapLeaves got odd number of iv pairs: %v", len(iv)))
	}
	r := []*trillian.MapLeaf{}
	var index []byte
	for i, b := range iv {
		if i%2 == 0 {
			index = b
			continue
		}
		r = append(r, &trillian.MapLeaf{
			Index:     index,
			LeafValue: b,
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
		trillian.HashStrategy
		index []byte
		value []byte
	}{
		{trillian.HashStrategy_TEST_MAP_HASHER, testonly.TransparentHash("A"), []byte("A")},
	} {
		tree, err := env.CreateMap(tc.HashStrategy)
		if err != nil {
			t.Errorf("CreateMap(): %v", err)
			continue
		}
		mapID := tree.TreeId
		hasher, err := hashers.NewMapHasher(tree.HashStrategy)
		if err != nil {
			t.Errorf("NewMapHasher(): %v", err)
			continue
		}
		client := trillian.NewTrillianMapClient(env.ClientConn)
		leaves := createMapLeaves(tc.index, tc.value)

		if _, err := client.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  mapID,
			Leaves: leaves,
		}); err != nil {
			t.Errorf("SetLeaves(): %v", err)
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
			t.Errorf("GetLeaves(): %v", err)
			continue
		}

		rootHash := getResp.GetMapRoot().GetRootHash()
		for _, m := range getResp.GetMapLeafInclusion() {
			index := m.GetLeaf().GetIndex()
			leafHash := m.GetLeaf().GetLeafHash()
			proof := m.GetInclusion()
			if err := merkle.VerifyMapInclusionProof(mapID, index,
				leafHash, rootHash, proof, hasher); err != nil {
				t.Errorf("VerifyMapInclusionProof(): %v", err)
			}
		}
	}
}

// h2b converts a hex string into []byte.
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}

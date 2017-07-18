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
	"bytes"
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
		desc         string
		HashStrategy trillian.HashStrategy
		iv           [][]byte
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

		leaves := createMapLeaves(tc.iv...)
		if _, err := env.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: leaves,
		}); err != nil {
			t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
			continue
		}

		indexes := [][]byte{}
		for _, l := range leaves {
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

		rootHash := getResp.GetMapRoot().GetRootHash()
		for _, m := range getResp.GetMapLeafInclusion() {
			index := m.GetLeaf().GetIndex()
			leafHash := m.GetLeaf().GetLeafHash()
			proof := m.GetInclusion()
			if err := merkle.VerifyMapInclusionProof(tree.TreeId, index,
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
	for _, tc := range []struct {
		desc                  string
		HashStrategy          trillian.HashStrategy
		batchSize, numBatches int
	}{
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    64, numBatches: 32,
		},
		// TODO(gdbelvin): investigate batches of size > 150.
		// We are currently getting DB connection starvation: Too many connections.
	} {
		tree, _, err := newTreeWithHasher(ctx, env, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}

		if err := RunMapBatchTest(ctx, env, tree, tc.batchSize, tc.numBatches); err != nil {
			t.Errorf("%v: %v", tc.desc, err)
		}
	}
}

func TestNonExistentLeaf(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, "TestNonExistentLeaf")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	for _, tc := range []struct {
		desc         string
		HashStrategy trillian.HashStrategy
		setIV        [][]byte
		getIndex     []byte
	}{
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			setIV: [][]byte{
				testonly.TransparentHash("A"), []byte("A"),
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
			Leaves: createMapLeaves(tc.setIV...),
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

			if got, want := leafHash,
				hasher.HashLeaf(tree.TreeId, index, hasher.BitLen(), leaf); !bytes.Equal(got, want) {
				t.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
			}
			if err := merkle.VerifyMapInclusionProof(tree.TreeId, index,
				leafHash, rootHash, proof, hasher); err != nil {
				t.Errorf("verifyMapInclusionProof(%x): %v", index, err)
			}
		}
	}
}

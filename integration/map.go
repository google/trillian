// Copyright 2016 Google Inc. All Rights Reserved.
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
	"crypto"
	"encoding/hex"
	"fmt"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

// createBatchLeaves produces n random i/v pairs
func createBatchLeaves(batch, n int) []*trillian.MapLeaf {
	leaves := make([]*trillian.MapLeaf, 0, n)
	for i := 0; i < n; i++ {
		leaves = append(leaves, &trillian.MapLeaf{
			Index:     testonly.HashKey(fmt.Sprintf("batch-%d-key-%d", batch, i)),
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

		if got, want := leafHash, hasher.HashLeaf(treeID, index, hasher.BitLen(), leaf); !bytes.Equal(got, want) {
			return fmt.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
		}
		if err := merkle.VerifyMapInclusionProof(treeID, index,
			leafHash, rootHash, proof, hasher); err != nil {
			return fmt.Errorf("verifyMapInclusionProof(%x): %v", index, err)
		}
	}
	return nil
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

	for _, indexes := range indexBatch {
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
			return err
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

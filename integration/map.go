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
	"encoding/hex"
	"fmt"
	"math/rand"

	"github.com/golang/glog"
	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
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
	for i := 0; i < len(iv); i += 2 {
		r = append(r, &trillian.MapLeaf{
			Index:     iv[i],
			LeafValue: iv[i+1],
		})
	}
	return r
}

// makeBatchIndexValues produces n random i/v pairs
func makeBatchIndexValues(batch, n int) [][]byte {
	iv := make([][]byte, 0, n*2)
	for i := 0; i < n; i++ {
		index := testonly.HashKey(fmt.Sprintf("batch-%d-key-%d", batch, i))
		value := []byte(fmt.Sprintf("batch-%d-value-%d", batch, i))
		iv = append(iv, index, value)
	}
	return iv
}

// RunMapIntegration runs a map integration test using the given map ID and client.
func RunMapIntegration(ctx context.Context, env *integration.MapEnv, tree *trillian.Tree) error {
	treeID := tree.TreeId
	pubKey, err := keys.NewFromPublicDER(tree.GetPublicKey().GetDer())
	if err != nil {
		return err
	}
	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return err
	}
	client := env.MapClient

	{
		// Ensure we're starting with an empty map
		r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: treeID})
		if err != nil {
			return fmt.Errorf("failed to get empty map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(0); got != want {
			return fmt.Errorf("got SMH with revision %d, want %d", got, want)
		}
	}

	// Generate tests.
	const batchSize = 64
	const numBatches = 32
	tests := createMapLeaves(makeBatchIndexValues(0, batchSize*numBatches)...)
	lookup := make(map[string]*trillian.MapLeaf)
	for i, t := range tests {
		lookup[hex.EncodeToString(t.Index)] = tests[i]
	}

	// Write some data in batches
	for x := 0; x < numBatches; x++ {
		glog.Infof("Starting batch %d...", x)

		req := &trillian.SetMapLeavesRequest{
			MapId:  treeID,
			Leaves: tests[x*batchSize : (x+1)*batchSize],
		}

		_, err := client.SetLeaves(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to write batch %d: %v", x, err)
		}
		glog.Infof("Set %d k/v pairs", len(req.Leaves))
	}

	// Check your head
	var latestRoot trillian.SignedMapRoot
	r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: treeID})
	if err != nil {
		return fmt.Errorf("failed to get map head: %v", err)
	}

	if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
		return fmt.Errorf("got SMH with revision %d, want %d", got, want)
	}
	latestRoot = *r.MapRoot

	// Check values
	// Mix up the ordering of requests
	randIndexes := make([][]byte, len(tests))
	for i, r := range rand.Perm(len(tests)) {
		randIndexes[i] = tests[r].Index
	}
	for i := 0; i < numBatches; i++ {
		getReq := &trillian.GetMapLeavesRequest{
			MapId:    treeID,
			Revision: -1,
			Index:    randIndexes[i*batchSize : (i+1)*batchSize],
		}

		r, err := client.GetLeaves(ctx, getReq)
		if err != nil {
			return fmt.Errorf("failed to get values: %v", err)
		}
		if got, want := len(r.MapLeafInclusion), len(getReq.Index); got != want {
			return fmt.Errorf("got %d values, want %d", got, want)
		}
		if got, want := r.GetMapRoot().GetMapRevision(), int64(numBatches); got != want {
			return fmt.Errorf("got SMH with revision %d, want %d", got, want)
		}
		// SignedMapRoot contains its own signature. To verify, we need to create a local
		// copy of the object and return the object to the state it was in when signed
		// by removing the signature from the object.
		smr := *r.GetMapRoot()
		smr.Signature = nil // Remove the signature from the object to be verified.
		if tcrypto.VerifyObject(pubKey, smr, r.GetMapRoot().GetSignature()) != nil {
			return fmt.Errorf("VerifyObject(SMR): %v", err)
		}
		rootHash := r.GetMapRoot().GetRootHash()
		for _, incl := range r.MapLeafInclusion {
			leaf := incl.GetLeaf().GetLeafValue()
			index := incl.GetLeaf().GetIndex()
			leafHash := incl.GetLeaf().GetLeafHash()
			proof := incl.GetInclusion()

			ev, ok := lookup[hex.EncodeToString(index)]
			if !ok {
				return fmt.Errorf("unexpected key returned: %s", index)
			}
			if got, want := leaf, ev.LeafValue; !bytes.Equal(got, want) {
				return fmt.Errorf("got value %s, want %s", got, want)
			}
			if got, want := hasher.HashLeaf(treeID, index, hasher.BitLen(), leaf), leafHash; !bytes.Equal(got, want) {
				return fmt.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
			}
			if err := merkle.VerifyMapInclusionProof(treeID, index,
				leafHash, rootHash, proof, hasher); err != nil {
				return fmt.Errorf("verifyMapInclusionProof(%x): %v", index, err)
			}
		}
	}
	if err := testForNonExistentLeaf(ctx, treeID, hasher, client, latestRoot); err != nil {
		return err
	}
	return nil
}

// Ensure that a query for a leaf that does not exist results in a valid inclusion proof.
func testForNonExistentLeaf(ctx context.Context, treeID int64, hasher hashers.MapHasher,
	client trillian.TrillianMapClient, latestRoot trillian.SignedMapRoot) error {
	index1 := []byte("doesnotexist....................")
	r, err := client.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
		MapId:    treeID,
		Revision: latestRoot.MapRevision,
		Index:    [][]byte{index1},
	})
	if err != nil {
		return fmt.Errorf("GetLeaves(%s): %v", index1, err)
	}
	for _, incl := range r.MapLeafInclusion {
		leaf := incl.Leaf
		if got, want := len(leaf.LeafValue), 0; got != want {
			return fmt.Errorf("len(GetLeaves(%s).LeafValue): %v, want, %v", index1, got, want)
		}
		leafHash := hasher.HashLeaf(treeID, leaf.Index, hasher.BitLen(), leaf.LeafValue)
		if err := merkle.VerifyMapInclusionProof(treeID, leaf.Index, leafHash, latestRoot.RootHash, incl.Inclusion, hasher); err != nil {
			return fmt.Errorf("VerifyMapInclusionProof(%x): %v", leaf.Index, err)
		}
	}
	return nil
}

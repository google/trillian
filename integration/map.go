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
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/testonly"
)

// RunMapIntegration runs a map integration test using the given map ID and client.
func RunMapIntegration(ctx context.Context, mapID int64, client trillian.TrillianMapClient) error {
	{
		// Ensure we're starting with an empty map
		r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: mapID})
		if err != nil {
			return fmt.Errorf("failed to get empty map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(0); got != want {
			return fmt.Errorf("got SMH with revision %d, expected %d", got, want)
		}
	}

	// Generate tests.
	const batchSize = 64
	const numBatches = 32
	tests := make([]*trillian.IndexValue, batchSize*numBatches)
	lookup := make(map[string]*trillian.IndexValue)
	for i := range tests {
		index := testonly.HashKey(fmt.Sprintf("key-%d", i))
		tests[i] = &trillian.IndexValue{
			Index: index,
			Value: &trillian.MapLeaf{
				LeafValue: []byte(fmt.Sprintf("value-%d", i)),
			},
		}
		lookup[hex.EncodeToString(index)] = tests[i]
	}

	// Write some data in batches
	for x := 0; x < numBatches; x++ {
		glog.Infof("Starting batch %d...", x)

		req := &trillian.SetMapLeavesRequest{
			MapId:      mapID,
			IndexValue: tests[x*batchSize : (x+1)*batchSize-1],
		}

		_, err := client.SetLeaves(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to write batch %d: %v", x, err)
		}
		glog.Infof("Set %d k/v pairs", len(req.IndexValue))
	}

	// Check your head
	var latestRoot trillian.SignedMapRoot
	r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: mapID})
	if err != nil {
		return fmt.Errorf("failed to get map head: %v", err)
	}

	if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
		return fmt.Errorf("got SMH with revision %d, expected %d", got, want)
	}
	// TODO(gbelvin) replace expected root test with proper inclusion tests.
	latestRoot = *r.MapRoot

	// Check values
	// Mix up the ordering of requests
	h := merkle.NewMapHasher(merkle.NewRFC6962TreeHasher(crypto.NewSHA256()))
	randIndexes := make([][]byte, len(tests))
	for i, r := range rand.Perm(len(tests)) {
		randIndexes[i] = tests[r].Index
	}
	for i := 0; i < numBatches; i++ {
		getReq := &trillian.GetMapLeavesRequest{
			MapId:    mapID,
			Revision: latestRoot.MapRevision,
			Index:    randIndexes[i*batchSize : (i+1)*batchSize-1],
		}

		r, err := client.GetLeaves(ctx, getReq)
		if err != nil {
			return fmt.Errorf("failed to get values: %v", err)
		}
		if got, want := len(r.IndexValue), len(getReq.Index); got != want {
			return fmt.Errorf("got %d values, expected %d", got, want)
		}
		for _, kv := range r.IndexValue {
			ev := lookup[hex.EncodeToString(kv.IndexValue.Index)]
			if ev == nil {
				return fmt.Errorf("unexpected key returned: %v", string(kv.IndexValue.Index))
			}
			if got, want := ev.Value.LeafValue,
				kv.IndexValue.Value.LeafValue; !bytes.Equal(got, want) {
				return fmt.Errorf("got value %x, expected %x", got, want)
			}
			leafHash := h.HashLeaf(kv.IndexValue.Value.LeafValue)
			proof := make([][]byte, len(kv.Inclusion))
			for i, v := range kv.Inclusion {
				proof[i] = v
			}
			if err := merkle.VerifyMapInclusionProof(kv.IndexValue.Index, leafHash, latestRoot.RootHash, proof, h); err != nil {
				return fmt.Errorf("inclusion proof failed to verify for key %s: %v", kv.IndexValue.Index, err)
			}
		}
	}
	return nil
}

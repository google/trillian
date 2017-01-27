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
	"encoding/base64"
	"fmt"
	"math/rand"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/testonly"
	"golang.org/x/net/context"
)

// RunMapIntegration runs a map integration test using the given map ID and client.
func RunMapIntegration(ctx context.Context, t *testing.T, mapID int64, client trillian.TrillianMapClient) {
	{
		// Ensure we're starting with an empty map
		r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: mapID})
		if err != nil {
			t.Fatalf("failed to get empty map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(0); got != want {
			t.Fatalf("got SMH with revision %d, expected %d", got, want)
		}
	}

	const batchSize = 64
	const numBatches = 32
	const expectedRootB64 = "XxWv/gFSjVVujxdCdDX4Z/GC/9JD8g/y8s1Ayf+boaE="
	expectedKeys := make([][]byte, 0, batchSize*numBatches)
	expectedValues := make(map[string][]byte)

	{
		// Write some data in batches
		rev := int64(0)
		var root []byte
		for x := 0; x < numBatches; x++ {
			glog.Infof("Starting batch %d...", x)

			req := &trillian.SetMapLeavesRequest{
				MapId:    mapID,
				KeyValue: make([]*trillian.KeyValue, batchSize),
			}

			for y := 0; y < batchSize; y++ {
				key := []byte(fmt.Sprintf("key-%d-%d", x, y))
				expectedKeys = append(expectedKeys, key)
				value := []byte(fmt.Sprintf("value-%d-%d", x, y))
				expectedValues[string(key)] = value
				req.KeyValue[y] = &trillian.KeyValue{
					Key: key,
					Value: &trillian.MapLeaf{
						LeafValue: value,
					},
				}
			}

			resp, err := client.SetLeaves(ctx, req)
			if err != nil {
				t.Fatalf("failed to write batch %d: %v", x, err)
			}
			glog.Infof("Set %d k/v pairs", len(req.KeyValue))
			root = resp.MapRoot.RootHash
			rev++
		}
		if expected, got := testonly.MustDecodeBase64(expectedRootB64), root; !bytes.Equal(expected, root) {
			t.Fatalf("expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}

	var latestRoot trillian.SignedMapRoot
	{
		// Check your head
		r, err := client.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: mapID})
		if err != nil {
			t.Fatalf("failed to get map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
			t.Fatalf("got SMH with revision %d, expected %d", got, want)
		}
		if expected, got := testonly.MustDecodeBase64(expectedRootB64), r.MapRoot.RootHash; !bytes.Equal(expected, got) {
			t.Fatalf("expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
		glog.Infof("Got expected roothash@%d: %s", r.MapRoot.MapRevision, base64.StdEncoding.EncodeToString(r.MapRoot.RootHash))
		latestRoot = *r.MapRoot
	}

	{
		// Check values
		getReq := trillian.GetMapLeavesRequest{
			MapId:    mapID,
			Revision: latestRoot.MapRevision,
		}
		// Mix up the ordering of requests
		keyOrder := rand.Perm(len(expectedKeys))
		i := 0

		h := merkle.NewMapHasher(merkle.NewRFC6962TreeHasher(crypto.NewSHA256()))

		for x := 0; x < numBatches; x++ {
			getReq.Key = make([][]byte, 0, batchSize)
			for y := 0; y < batchSize; y++ {
				getReq.Key = append(getReq.Key, expectedKeys[keyOrder[i]])
				i++
			}
			r, err := client.GetLeaves(ctx, &getReq)
			if err != nil {
				t.Fatalf("failed to get values: %v", err)
			}
			if got, want := len(r.KeyValue), len(getReq.Key); got != want {
				t.Fatalf("got %d values, expected %d", got, want)
			}
			for _, kv := range r.KeyValue {
				ev := expectedValues[string(kv.KeyValue.Key)]
				if ev == nil {
					t.Fatalf("unexpected key returned: %v", string(kv.KeyValue.Key))
				}
				if got, want := ev, kv.KeyValue.Value.LeafValue; !bytes.Equal(got, want) {
					t.Fatalf("got value %x, expected %x", got, want)
				}
				keyHash := h.HashKey(kv.KeyValue.Key)
				leafHash := h.HashLeaf(kv.KeyValue.Value.LeafValue)
				proof := make([][]byte, len(kv.Inclusion))
				for i, v := range kv.Inclusion {
					proof[i] = v
				}
				if err := merkle.VerifyMapInclusionProof(keyHash, leafHash, latestRoot.RootHash, proof, h); err != nil {
					t.Fatalf("inclusion proof failed to verify for key %s: %v", kv.KeyValue.Key, err)
				}
				delete(expectedValues, string(kv.KeyValue.Key))
			}
		}
		if got := len(expectedValues); got != 0 {
			t.Fatalf("still have %d unmatched expected values remaining", got)
		}

	}
}

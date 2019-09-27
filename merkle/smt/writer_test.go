// Copyright 2019 Google Inc. All Rights Reserved.
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

package smt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"

	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
	"golang.org/x/sync/errgroup"
)

const treeID = int64(0)

var (
	hasher = maphasher.Default
	b64    = testonly.MustDecodeBase64
)

func TestWriter(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		upd      []NodeUpdate
		wantRoot []byte
	}{
		{
			// Taken from SparseMerkleTreeWriter tests.
			desc:     "non-empty",
			upd:      []NodeUpdate{genUpd("key1", "value1"), genUpd("key2", "value2"), genUpd("key3", "value3")},
			wantRoot: b64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			w := NewWriter(treeID, hasher, 256, 0)
			rootUpd, err := w.Write(tc.upd, noopAccessor{})
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if got, want := rootUpd.Hash, tc.wantRoot; !bytes.Equal(got, want) {
				t.Errorf("root mismatch: got %x, want %x", got, want)
			}
		})
	}
}

func TestWriterBigBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("BigBatch test is not short")
	}

	const batchSize = 1024
	const numBatches = 4
	upd := make([]NodeUpdate, 0, batchSize*numBatches)
	for x := 0; x < numBatches; x++ {
		for y := 0; y < batchSize; y++ {
			u := genUpd(fmt.Sprintf("key-%d-%d", x, y), fmt.Sprintf("value-%d-%d", x, y))
			upd = append(upd, u)
		}
	}

	w := NewWriter(treeID, hasher, 256, 8)
	shards, err := w.Split(upd)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	var mu sync.Mutex
	splitUpd := make([]NodeUpdate, 0, 256)

	eg, _ := errgroup.WithContext(context.Background())
	for _, upd := range shards {
		upd := upd
		eg.Go(func() error {
			rootUpd, err := w.Write(upd, noopAccessor{})
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			splitUpd = append(splitUpd, rootUpd)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	rootUpd, err := w.Write(splitUpd, noopAccessor{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Calculated using Python code.
	want := b64("Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA=")
	if got := rootUpd.Hash; !bytes.Equal(got, want) {
		t.Errorf("root mismatch: got %x, want %x", got, want)
	}
}

// genUpd returns a NodeUpdate for the given key and value. The returned node
// ID is 256-bit map key based on SHA256 of the given key string.
func genUpd(key, value string) NodeUpdate {
	key256 := sha256.Sum256([]byte(key))
	hash := hasher.HashLeaf(treeID, key256[:], []byte(value))
	return NodeUpdate{ID: tree.NewNodeID2(string(key256[:]), 256), Hash: hash}
}

type noopAccessor struct{}

func (n noopAccessor) Get([]tree.NodeID2) (map[tree.NodeID2][]byte, error) {
	return make(map[tree.NodeID2][]byte), nil
}

func (n noopAccessor) Set([]NodeUpdate) error { return nil }

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
	"errors"
	"fmt"
	"reflect"
	"strings"
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

func TestWriterSplit(t *testing.T) {
	ids := []tree.NodeID2{
		tree.NewNodeID2("\x01\x00\x00\x00", 32),
		tree.NewNodeID2("\x00\x00\x00\x00", 32),
		tree.NewNodeID2("\x02\x00\x00\x00", 32),
		tree.NewNodeID2("\x03\x00\x00\x00", 32),
		tree.NewNodeID2("\x02\x00\x01\x00", 32),
		tree.NewNodeID2("\x03\x00\x00\x00", 32),
	}
	// Generate some node updates based on IDs.
	upd := make([]NodeUpdate, len(ids))
	for i, id := range ids {
		upd[i] = NodeUpdate{ID: id, Hash: []byte(fmt.Sprintf("%32d", i))}
	}

	for _, tc := range []struct {
		desc  string
		split uint
		upd   []NodeUpdate
		want  [][]NodeUpdate
		err   bool
	}{
		{desc: "dup", upd: upd, err: true},
		{desc: "wrong-len", upd: []NodeUpdate{{ID: tree.NewNodeID2("ab", 10)}}, err: true},
		{desc: "ok-24", split: 24, upd: upd[:5],
			want: [][]NodeUpdate{{upd[1]}, {upd[0]}, {upd[2]}, {upd[4]}, {upd[3]}}},
		{desc: "ok-21", split: 21, upd: upd[:5],
			want: [][]NodeUpdate{{upd[1]}, {upd[0]}, {upd[2], upd[4]}, {upd[3]}}},
		{desc: "ok-16", split: 16, upd: upd[:5],
			want: [][]NodeUpdate{{upd[1]}, {upd[0]}, {upd[2], upd[4]}, {upd[3]}}},
		{desc: "ok-0", split: 0, upd: upd[:5],
			want: [][]NodeUpdate{{upd[1], upd[0], upd[2], upd[4], upd[3]}}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			upd := make([]NodeUpdate, len(tc.upd))
			copy(upd, tc.upd) // Avoid shuffling effects.

			w := NewWriter(treeID, hasher, 32, tc.split)
			shards, err := w.Split(upd)
			if !reflect.DeepEqual(shards, tc.want) {
				t.Error("shards mismatch")
			}
			if got, want := err != nil, tc.err; got != want {
				t.Errorf("got err: %v, want %v", err, want)
			}
		})
	}
}

func TestWriterWrite(t *testing.T) {
	ctx := context.Background()
	upd := []NodeUpdate{genUpd("key1", "value1"), genUpd("key2", "value2"), genUpd("key3", "value3")}
	for _, tc := range []struct {
		desc     string
		split    uint
		acc      noopAccessor
		upd      []NodeUpdate
		wantRoot []byte
		wantErr  string
	}{
		// Taken from SparseMerkleTreeWriter tests.
		{
			desc:     "single-leaf",
			upd:      []NodeUpdate{upd[0]},
			wantRoot: b64("PPI818D5CiUQQMZulH58LikjxeOFWw2FbnGM0AdVHWA="),
		},
		{
			desc:     "multi-leaf",
			upd:      []NodeUpdate{upd[0], upd[1], upd[2]},
			wantRoot: b64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
		},

		{desc: "empty", wantErr: "nothing to write"},
		{desc: "unaligned", upd: []NodeUpdate{{ID: tree.NewNodeID2("ab", 10)}}, wantErr: "unexpected depth"},
		{desc: "dup", upd: []NodeUpdate{upd[0], upd[0]}, wantErr: "duplicate ID"},
		{desc: "2-shards", split: 128, upd: []NodeUpdate{upd[0], upd[1]}, wantErr: "writing across"},
		{desc: "get-err", acc: noopAccessor{get: errors.New("nope")}, upd: []NodeUpdate{upd[0]}, wantErr: "nope"},
		{desc: "set-err", acc: noopAccessor{set: errors.New("nope")}, upd: []NodeUpdate{upd[0]}, wantErr: "nope"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			w := NewWriter(treeID, hasher, 256, tc.split)
			rootUpd, err := w.Write(ctx, tc.upd, tc.acc)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if got, want := gotErr, tc.wantErr; !strings.Contains(got, want) {
				t.Errorf("Write: want err containing %q, got %v", want, err)
			}
			if got, want := rootUpd.Hash, tc.wantRoot; !bytes.Equal(got, want) {
				t.Errorf("Write: got root %x, want %x", got, want)
			}
		})
	}
}

func TestWriterBigBatch(t *testing.T) {
	testWriterBigBatch(t)
}

func BenchmarkWriterBigBatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testWriterBigBatch(b)
	}
}

func testWriterBigBatch(t testing.TB) {
	if testing.Short() {
		t.Skip("BigBatch test is not short")
	}
	ctx := context.Background()

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
			rootUpd, err := w.Write(ctx, upd, noopAccessor{})
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

	rootUpd, err := w.Write(ctx, splitUpd, noopAccessor{})
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

type noopAccessor struct {
	get, set error
}

func (n noopAccessor) Get(context.Context, []tree.NodeID2) (map[tree.NodeID2][]byte, error) {
	if err := n.get; err != nil {
		return nil, err
	}
	return make(map[tree.NodeID2][]byte), nil
}

func (n noopAccessor) Set(context.Context, []NodeUpdate) error { return n.set }

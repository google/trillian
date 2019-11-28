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
	upd := make([]Node, len(ids))
	for i, id := range ids {
		upd[i] = Node{ID: id, Hash: []byte(fmt.Sprintf("%32d", i))}
	}

	for _, tc := range []struct {
		desc  string
		split uint
		upd   []Node
		want  [][]Node
		err   bool
	}{
		{desc: "dup", upd: upd, err: true},
		{desc: "wrong-len", upd: []Node{{ID: tree.NewNodeID2("ab", 10)}}, err: true},
		{desc: "ok-24", split: 24, upd: upd[:5],
			want: [][]Node{{upd[1]}, {upd[0]}, {upd[2]}, {upd[4]}, {upd[3]}}},
		{desc: "ok-21", split: 21, upd: upd[:5],
			want: [][]Node{{upd[1]}, {upd[0]}, {upd[2], upd[4]}, {upd[3]}}},
		{desc: "ok-16", split: 16, upd: upd[:5],
			want: [][]Node{{upd[1]}, {upd[0]}, {upd[2], upd[4]}, {upd[3]}}},
		{desc: "ok-0", split: 0, upd: upd[:5],
			want: [][]Node{{upd[1], upd[0], upd[2], upd[4], upd[3]}}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			upd := make([]Node, len(tc.upd))
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
	upd := []Node{genUpd("key1", "value1"), genUpd("key2", "value2"), genUpd("key3", "value3")}
	for _, tc := range []struct {
		desc     string
		split    uint
		acc      *testAccessor
		upd      []Node
		wantRoot []byte
		wantErr  string
	}{
		// Taken from SparseMerkleTreeWriter tests.
		{
			desc:     "single-leaf",
			upd:      []Node{upd[0]},
			wantRoot: b64("PPI818D5CiUQQMZulH58LikjxeOFWw2FbnGM0AdVHWA="),
		},
		{
			desc:     "multi-leaf",
			upd:      []Node{upd[0], upd[1], upd[2]},
			wantRoot: b64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
		},

		{desc: "empty", wantErr: "nothing to write"},
		{desc: "unaligned", upd: []Node{{ID: tree.NewNodeID2("ab", 10)}}, wantErr: "unexpected depth"},
		{desc: "dup", upd: []Node{upd[0], upd[0]}, wantErr: "duplicate ID"},
		{desc: "2-shards", split: 128, upd: []Node{upd[0], upd[1]}, wantErr: "writing across"},
		{desc: "get-err", acc: &testAccessor{get: errors.New("fail")}, upd: []Node{upd[0]}, wantErr: "fail"},
		{desc: "set-err", acc: &testAccessor{set: errors.New("fail")}, upd: []Node{upd[0]}, wantErr: "fail"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			w := NewWriter(treeID, hasher, 256, tc.split)
			acc := tc.acc
			if acc == nil {
				acc = &testAccessor{}
			}
			rootUpd, err := w.Write(ctx, tc.upd, acc)
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
	upd := make([]Node, 0, batchSize*numBatches)
	for x := 0; x < numBatches; x++ {
		for y := 0; y < batchSize; y++ {
			u := genUpd(fmt.Sprintf("key-%d-%d", x, y), fmt.Sprintf("value-%d-%d", x, y))
			upd = append(upd, u)
		}
	}

	w := NewWriter(treeID, hasher, 256, 8)
	rootUpd := update(ctx, t, w, &testAccessor{}, upd)

	// Calculated using Python code from the original Revocation Transparency
	// doc: https://www.links.org/files/RevocationTransparency.pdf.
	want := b64("Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA=")
	if got := rootUpd.Hash; !bytes.Equal(got, want) {
		t.Errorf("root mismatch: got %x, want %x", got, want)
	}
}

func TestWriterBigBatchMultipleWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("BigBatch test is not short")
	}
	ctx := context.Background()

	const batchSize = 1024
	const numBatches = 4
	roots := [numBatches][]byte{
		b64("7R5uvGy5MJ2Y8xrQr4/mnn3aPw39vYscghmg9KBJaKc="),
		b64("VTrPStz/chupeOjzAYFIHGfhiMT8yN+v589jxWZO1F0="),
		b64("nRvRV/NfC06rXGI5cKeTieyyp/69bHoMcVDs0AtZzus="),
		b64("Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA="),
	}

	w := NewWriter(treeID, hasher, 256, 8)
	acc := &testAccessor{h: make(map[tree.NodeID2][]byte), save: true}

	for i := 0; i < numBatches; i++ {
		upd := make([]Node, 0, batchSize)
		for j := 0; j < batchSize; j++ {
			u := genUpd(fmt.Sprintf("key-%d-%d", i, j), fmt.Sprintf("value-%d-%d", i, j))
			upd = append(upd, u)
		}
		rootUpd := update(ctx, t, w, acc, upd)
		if got, want := rootUpd.Hash, roots[i]; !bytes.Equal(got, want) {
			t.Errorf("%d: root mismatch: got %x, want %x", i, got, want)
		}
	}
}

func update(ctx context.Context, t testing.TB, w *Writer, acc NodeBatchAccessor, upd []Node) Node {
	shards, err := w.Split(upd)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	var mu sync.Mutex
	splitUpd := make([]Node, 0, 256)

	eg, _ := errgroup.WithContext(ctx)
	for _, upd := range shards {
		upd := upd
		eg.Go(func() error {
			rootUpd, err := w.Write(ctx, upd, acc)
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

	rootUpd, err := w.Write(ctx, splitUpd, acc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return rootUpd
}

// genUpd returns a Node for the given key and value. The returned node ID is
// 256-bit map key based on SHA256 of the given key string.
func genUpd(key, value string) Node {
	key256 := sha256.Sum256([]byte(key))
	hash := hasher.HashLeaf(treeID, key256[:], []byte(value))
	return Node{ID: tree.NewNodeID2(string(key256[:]), 256), Hash: hash}
}

// testAccessor implements NodeBatchAccessor for testing purposes.
type testAccessor struct {
	mu   sync.RWMutex // Guards the h map.
	h    map[tree.NodeID2][]byte
	save bool  // Persist node updates in this accessor.
	get  error // The error returned by Get.
	set  error // The error returned by Set.
}

func (t *testAccessor) Get(ctx context.Context, ids []tree.NodeID2) (map[tree.NodeID2][]byte, error) {
	if err := t.get; err != nil {
		return nil, err
	} else if !t.save {
		return nil, nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	h := make(map[tree.NodeID2][]byte, len(ids))
	for _, id := range ids {
		if hash, ok := t.h[id]; ok {
			h[id] = hash
		}
	}
	return h, nil
}

func (t *testAccessor) Set(ctx context.Context, upd []Node) error {
	if err := t.set; err != nil {
		return err
	} else if !t.save {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, u := range upd {
		t.h[u.ID] = u.Hash
	}
	return nil
}

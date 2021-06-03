// Copyright 2019 Google LLC. All Rights Reserved.
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

	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
	"golang.org/x/sync/errgroup"
)

const treeID = int64(0)

var (
	hasher = coniks.Default
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
	// Generate some nodes based on IDs.
	all := make([]Node, len(ids))
	for i, id := range ids {
		all[i] = Node{ID: id, Hash: []byte(fmt.Sprintf("%32d", i))}
	}

	for _, tc := range []struct {
		desc  string
		split uint
		nodes []Node
		want  [][]Node
		err   bool
	}{
		{desc: "dup", nodes: all, err: true},
		{desc: "wrong-len", nodes: []Node{{ID: tree.NewNodeID2("ab", 10)}}, err: true},
		{
			desc: "ok-24", split: 24, nodes: all[:5],
			want: [][]Node{{all[1]}, {all[0]}, {all[2]}, {all[4]}, {all[3]}},
		},
		{
			desc: "ok-21", split: 21, nodes: all[:5],
			want: [][]Node{{all[1]}, {all[0]}, {all[2], all[4]}, {all[3]}},
		},
		{
			desc: "ok-16", split: 16, nodes: all[:5],
			want: [][]Node{{all[1]}, {all[0]}, {all[2], all[4]}, {all[3]}},
		},
		{
			desc: "ok-0", split: 0, nodes: all[:5],
			want: [][]Node{{all[1], all[0], all[2], all[4], all[3]}},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			nodes := make([]Node, len(tc.nodes))
			copy(nodes, tc.nodes) // Avoid shuffling effects.

			w := NewWriter(treeID, hasher, 32, tc.split)
			shards, err := w.Split(nodes)
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
	all := []Node{genNode("key1", "value1"), genNode("key2", "value2"), genNode("key3", "value3")}
	for _, tc := range []struct {
		desc     string
		split    uint
		acc      *testAccessor
		nodes    []Node
		wantRoot []byte
		wantErr  string
	}{
		// Taken from SparseMerkleTreeWriter tests.
		{
			desc:     "single-leaf",
			nodes:    []Node{all[0]},
			wantRoot: b64("KKBw4yrtfa5Ugm9jo4SHZ79wJy4bUAW1jLPyMOJoAPQ="),
		},
		{
			desc:     "multi-leaf",
			nodes:    []Node{all[0], all[1], all[2]},
			wantRoot: b64("l/IJC6+DcqpNvnQ+HdAwwmXEXfmQ6Ha9/lOD7smeIVc="),
		},

		{desc: "empty", wantErr: "nothing to write"},
		{desc: "unaligned", nodes: []Node{{ID: tree.NewNodeID2("ab", 10)}}, wantErr: "unexpected depth"},
		{desc: "dup", nodes: []Node{all[0], all[0]}, wantErr: "duplicate ID"},
		{desc: "2-shards", split: 128, nodes: []Node{all[0], all[1]}, wantErr: "writing across"},
		{desc: "get-err", acc: &testAccessor{get: errors.New("fail")}, nodes: []Node{all[0]}, wantErr: "fail"},
		{desc: "set-err", acc: &testAccessor{set: errors.New("fail")}, nodes: []Node{all[0]}, wantErr: "fail"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			w := NewWriter(treeID, hasher, 256, tc.split)
			acc := tc.acc
			if acc == nil {
				acc = &testAccessor{}
			}
			rootUpd, err := w.Write(ctx, tc.nodes, acc)
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
	nodes := make([]Node, 0, batchSize*numBatches)
	for x := 0; x < numBatches; x++ {
		for y := 0; y < batchSize; y++ {
			u := genNode(fmt.Sprintf("key-%d-%d", x, y), fmt.Sprintf("value-%d-%d", x, y))
			nodes = append(nodes, u)
		}
	}

	w := NewWriter(treeID, hasher, 256, 8)
	rootUpd := update(ctx, t, w, &testAccessor{}, nodes)

	// Calculated using Python code from the original Revocation Transparency
	// doc: https://www.links.org/files/RevocationTransparency.pdf, but using the
	// CONIKS hasher instead.
	want := b64("P2SiYPpD858dVfAIG5RW0dxKKm7ZQr6DrhVIMDBWcJY=")
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
		b64("aAMq3tm3aChuTrjocEp9pau/rERbY3ClQ5iLuvkOwAw="),
		b64("8F5CF69Dkhebse22dhPvmwxaXGESqtKfQB3A8rMLh9k="),
		b64("f1b6zuA5OuG2Joedcq0XYm9AwGUw//C2ZAyGxqOv+G4="),
		b64("P2SiYPpD858dVfAIG5RW0dxKKm7ZQr6DrhVIMDBWcJY="),
	}

	w := NewWriter(treeID, hasher, 256, 8)
	acc := &testAccessor{h: make(map[tree.NodeID2][]byte), save: true}

	for i := 0; i < numBatches; i++ {
		nodes := make([]Node, 0, batchSize)
		for j := 0; j < batchSize; j++ {
			u := genNode(fmt.Sprintf("key-%d-%d", i, j), fmt.Sprintf("value-%d-%d", i, j))
			nodes = append(nodes, u)
		}
		rootUpd := update(ctx, t, w, acc, nodes)
		if got, want := rootUpd.Hash, roots[i]; !bytes.Equal(got, want) {
			t.Errorf("%d: root mismatch: got %x, want %x", i, got, want)
		}
	}
}

func update(ctx context.Context, t testing.TB, w *Writer, acc NodeBatchAccessor, nodes []Node) Node {
	shards, err := w.Split(nodes)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	var mu sync.Mutex
	splitUpd := make([]Node, 0, 256)

	eg, _ := errgroup.WithContext(ctx)
	for _, nodes := range shards {
		nodes := nodes
		eg.Go(func() error {
			rootUpd, err := w.Write(ctx, nodes, acc)
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

// genNode returns a Node for the given key and value. The returned node ID is
// 256-bit map key based on SHA256 of the given key string.
func genNode(key, value string) Node {
	key256 := sha256.Sum256([]byte(key))
	id := tree.NewNodeID2(string(key256[:]), uint(len(key256)*8))
	hash := hasher.HashLeaf(treeID, id, []byte(value))
	return Node{ID: id, Hash: hash}
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

func (t *testAccessor) Set(ctx context.Context, nodes []Node) error {
	if err := t.set; err != nil {
		return err
	} else if !t.save {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, n := range nodes {
		t.h[n.ID] = n.Hash
	}
	return nil
}

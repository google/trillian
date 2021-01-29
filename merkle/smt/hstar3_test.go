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
	"crypto"
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	coniks "github.com/google/trillian/merkle/coniks/hasher"
	"github.com/google/trillian/storage/tree"
)

type emptyNodes struct {
	h   mapHasher
	ids map[tree.NodeID2]bool
}

func (e *emptyNodes) Get(id tree.NodeID2) ([]byte, error) {
	if e.ids != nil {
		if !e.ids[id] {
			return nil, fmt.Errorf("not found or read twice: %v", id)
		}
		delete(e.ids, id) // Allow getting this ID only once.
	}
	return e.h.hashEmpty(id), nil
}

func (e *emptyNodes) Set(id tree.NodeID2, hash []byte) {}

func BenchmarkHStar3Root(b *testing.B) {
	hasher := coniks.New(crypto.SHA256)
	for i := 0; i < b.N; i++ {
		nodes := leafNodes(b, 512)
		hs, err := NewHStar3(nodes, hasher.HashChildren, 256, 0)
		if err != nil {
			b.Fatalf("NewHStar3: %v", err)
		}
		acc := &emptyNodes{h: bindHasher(hasher, 42)}
		if _, err := hs.Update(acc); err != nil {
			b.Fatalf("Update: %v", err)
		}
	}
}

// This test checks HStar3 implementation against HStar2-generated result.
func TestHStar3Golden(t *testing.T) {
	hasher := coniks.New(crypto.SHA256)
	nodes := leafNodes(t, 500)
	hs, err := NewHStar3(nodes, hasher.HashChildren, 256, 0)
	if err != nil {
		t.Fatalf("NewHStar3: %v", err)
	}
	acc := &emptyNodes{h: bindHasher(hasher, 42)}
	rootNodes, err := hs.Update(acc)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ln := len(rootNodes); ln != 1 {
		t.Fatalf("Update returned %d nodes, want 1", ln)
	}

	want := "daf17dc2c83f37962bae8a65d294ef7fca4ffa02c10bdc4ca5c4dec408001c98"
	if got := hex.EncodeToString(rootNodes[0].Hash); got != want {
		t.Errorf("Root: got %x, want %v", rootNodes[0].Hash, want)
	}
}

func TestNewHStar3(t *testing.T) {
	id1 := tree.NewNodeID2("01234567890000000000000000000001", 256)
	id2 := tree.NewNodeID2("01234567890000000000000000000002", 256)
	id3 := tree.NewNodeID2("01234567890000000000000000000003", 256)
	id4 := tree.NewNodeID2("01234567890000000000000001111111", 256)
	hasher := coniks.Default

	for _, tc := range []struct {
		desc    string
		nodes   []Node
		top     uint
		want    []Node
		wantErr string
	}{
		{desc: "depth-err", nodes: []Node{{ID: id1.Prefix(10)}}, wantErr: "invalid depth"},
		{desc: "dup-err1", nodes: []Node{{ID: id1}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "dup-err2", nodes: []Node{{ID: id1}, {ID: id2}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "top-vs-depth-err", nodes: []Node{{ID: id1}}, top: 300, wantErr: "top > depth"},
		{
			desc:  "ok1",
			nodes: []Node{{ID: id2}, {ID: id1}, {ID: id4}, {ID: id3}},
			want:  []Node{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
		{
			desc:  "ok2",
			nodes: []Node{{ID: id4}, {ID: id3}, {ID: id2}, {ID: id1}},
			want:  []Node{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			nodes := tc.nodes // No need to copy it here.
			// Note: NewHStar3 potentially shuffles nodes.
			_, err := NewHStar3(tc.nodes, hasher.HashChildren, 256, tc.top)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if want := tc.wantErr; !strings.Contains(got, want) {
				t.Errorf("NewHStar3: want error containing %q, got %v", want, err)
			}
			if want := tc.want; want != nil && !reflect.DeepEqual(nodes, want) {
				t.Errorf("NewHStar3: want nodes:\n%v\ngot:\n%v", nodes, want)
			}
		})
	}
}

func TestHStar3Prepare(t *testing.T) {
	hasher := coniks.Default
	nodes := leafNodes(t, 512)
	hs, err := NewHStar3(nodes, hasher.HashChildren, 256, 0)
	if err != nil {
		t.Fatalf("NewHStar3: %v", err)
	}
	rs := idsToMap(t, hs.Prepare())

	acc := &emptyNodes{h: bindHasher(hasher, 42), ids: rs}
	if _, err = hs.Update(acc); err != nil {
		t.Errorf("Update: %v", err)
	}
	if got := len(acc.ids); got != 0 {
		t.Errorf("%d ids were not read", got)
	}
}

func TestHStar3PrepareAlternative(t *testing.T) {
	// This is the intuitively simpler alternative Prepare implementation.
	prepare := func(nodes []Node, depth, top uint) map[tree.NodeID2]bool {
		ids := make(map[tree.NodeID2]bool)
		// For each node, add all its ancestors' siblings, down to the given depth.
		for _, n := range nodes {
			for id, d := n.ID, depth; d > top; d-- {
				pref := id.Prefix(d)
				if _, ok := ids[pref]; ok {
					// Delete the prefix node because its original hash does not contribute
					// to the updates, so should not be read.
					delete(ids, pref)
					// All the upper siblings have been added already, so skip them.
					break
				}
				ids[pref.Sibling()] = true
			}
		}
		return ids
	}

	for n := 0; n <= 32; n++ {
		t.Run(fmt.Sprintf("n:%d", n), func(t *testing.T) {
			nodes := leafNodes(t, n)
			hs, err := NewHStar3(nodes, nil, 256, 8)
			if err != nil {
				t.Fatalf("NewHStar3: %v", err)
			}
			ids := prepare(nodes, 256, 8)
			got := idsToMap(t, hs.Prepare())
			if !reflect.DeepEqual(got, ids) {
				t.Error("IDs mismatch")
			}
		})
	}
}

func BenchmarkHStar3Prepare(b *testing.B) {
	for i := 0; i < b.N; i++ {
		nodes := leafNodes(b, 512)
		hs, err := NewHStar3(nodes, nil, 256, 0)
		if err != nil {
			b.Fatalf("NewHStar3: %v", err)
		}
		_ = hs.Prepare()
	}
}

// leafNodes generates n pseudo-random leaf nodes at depth 256. The returned
// data depends only on n. The algorithm is the same as in HStar2 tests, which
// allows cross-checking their results.
func leafNodes(t testing.TB, n int) []Node {
	t.Helper()
	// Use a random sequence that depends on n.
	r := rand.New(rand.NewSource(int64(n)))
	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i].Hash = make([]byte, 32)
		if _, err := r.Read(nodes[i].Hash); err != nil {
			t.Fatalf("Failed to make random leaf hash: %v", err)
		}
		path := make([]byte, 32)
		if _, err := r.Read(path); err != nil {
			t.Fatalf("Failed to make random path: %v", err)
		}
		nodes[i].ID = tree.NewNodeID2(string(path), 256)
	}

	return nodes
}

func idsToMap(t testing.TB, ids []tree.NodeID2) map[tree.NodeID2]bool {
	t.Helper()
	res := make(map[tree.NodeID2]bool, len(ids))
	for _, id := range ids {
		if res[id] {
			t.Errorf("ID duplicate: %v", id)
		}
		res[id] = true
	}
	return res
}

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
	"crypto"
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/tree"
)

type emptyNodes struct {
	treeID int64
	hasher hashers.MapHasher
	hashes map[tree.NodeID2][]byte
}

func (e *emptyNodes) Get(id tree.NodeID2) ([]byte, error) {
	if e.hashes != nil {
		if _, ok := e.hashes[id]; !ok {
			return nil, fmt.Errorf("not found or read twice: %v", id)
		}
		delete(e.hashes, id) // Allow getting this ID only once.
	}
	index := make([]byte, e.hasher.Size())
	copy(index, id.FullBytes())
	if last, bits := id.LastByte(); bits != 0 {
		index[len(id.FullBytes())] = last
	}
	// TODO(pavelkalinnikov): Make HashEmpty method take the id directly.
	return e.hasher.HashEmpty(e.treeID, index, e.hasher.BitLen()-int(id.BitLen())), nil
}

func (e *emptyNodes) Set(id tree.NodeID2, hash []byte) {}

func BenchmarkHStar3Root(b *testing.B) {
	hasher := coniks.New(crypto.SHA256)
	for i := 0; i < b.N; i++ {
		updates := leafUpdates(b, 512)
		hs := NewHStar3(hasher.HashChildren, 256)
		if err := hs.Prepare(updates); err != nil {
			b.Fatalf("Prepare: %v", err)
		}
		nodes := &emptyNodes{treeID: 42, hasher: hasher}
		if _, err := hs.Update(updates, 0, nodes); err != nil {
			b.Fatalf("Update: %v", err)
		}
	}
}

// This test checks HStar3 implementation against HStar2-generated result.
func TestHStar3Golden(t *testing.T) {
	hasher := coniks.New(crypto.SHA256)
	hs := NewHStar3(hasher.HashChildren, 256)
	updates := leafUpdates(t, 500)
	if err := hs.Prepare(updates); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	nodes := &emptyNodes{treeID: 42, hasher: hasher}
	upd, err := hs.Update(updates, 0, nodes)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ln := len(upd); ln != 1 {
		t.Fatalf("Update returned %d updates, want 1", ln)
	}

	want := "d01bd540dc8b4a3ca3ac8deb485b431e9ce1290becb36838c4463a811d15c7f6"
	if got := hex.EncodeToString(upd[0].Hash); got != want {
		t.Errorf("Root: got %x, want %v", upd[0].Hash, want)
	}
}

func TestHStar3Prepare(t *testing.T) {
	id1 := tree.NewNodeID2("01234567890000000000000000000001", 256)
	id2 := tree.NewNodeID2("01234567890000000000000000000002", 256)
	id3 := tree.NewNodeID2("01234567890000000000000000000003", 256)
	id4 := tree.NewNodeID2("01234567890000000000000001111111", 256)
	hasher := coniks.Default
	hs := NewHStar3(hasher.HashChildren, 256)

	for _, tc := range []struct {
		desc    string
		upd     []NodeUpdate
		want    []NodeUpdate
		wantErr string
	}{
		{desc: "depth-err", upd: []NodeUpdate{{ID: id1.Prefix(10)}}, wantErr: "invalid depth"},
		{desc: "dup-err1", upd: []NodeUpdate{{ID: id1}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "dup-err2", upd: []NodeUpdate{{ID: id1}, {ID: id2}, {ID: id1}}, wantErr: "duplicate ID"},
		{
			desc: "ok1",
			upd:  []NodeUpdate{{ID: id2}, {ID: id1}, {ID: id4}, {ID: id3}},
			want: []NodeUpdate{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
		{
			desc: "ok2",
			upd:  []NodeUpdate{{ID: id4}, {ID: id3}, {ID: id2}, {ID: id1}},
			want: []NodeUpdate{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			upd := tc.upd          // No need to copy it here.
			err := hs.Prepare(upd) // Potentially shuffles upd.
			got := ""
			if err != nil {
				got = err.Error()
			}
			if want := tc.wantErr; !strings.Contains(got, want) {
				t.Errorf("Prepare: want error containing %q, got %v", want, err)
			}
			if want := tc.want; want != nil && !reflect.DeepEqual(upd, want) {
				t.Errorf("Prepare: want updates:\n%v\ngot:\n%v", upd, want)
			}
		})
	}
}

func TestHStar3GetReadSet(t *testing.T) {
	hasher := coniks.Default
	hs := NewHStar3(hasher.HashChildren, 256)

	updates := leafUpdates(t, 512)
	if err := hs.Prepare(updates); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	rs := hs.GetReadSet(updates, 0)

	nodes := &emptyNodes{treeID: 42, hasher: hasher, hashes: rs}
	_, err := hs.Update(updates, 0, nodes)
	if err != nil {
		t.Errorf("Update: %v", err)
	}
	if got := len(nodes.hashes); got != 0 {
		t.Errorf("%d hashes were not read", got)
	}
}

func leafUpdates(t testing.TB, n int) []NodeUpdate {
	t.Helper()
	updates := make([]NodeUpdate, n)

	// Use a fixed sequence to ensure runs are comparable.
	r := rand.New(rand.NewSource(42424242))
	for i := range updates {
		updates[i].Hash = make([]byte, 32)
		if _, err := r.Read(updates[i].Hash); err != nil {
			t.Fatalf("Failed to make random leaf hash: %v", err)
		}
		path := make([]byte, 32)
		if _, err := r.Read(path); err != nil {
			t.Fatalf("Failed to make random path: %v", err)
		}
		updates[i].ID = tree.NewNodeID2(string(path), 256)
	}

	return updates
}

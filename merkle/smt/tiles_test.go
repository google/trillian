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
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage/tree"
)

func TestTileSetAdd(t *testing.T) {
	hasher := maphasher.Default
	l := tree.NewLayout([]int{8, 8})

	existing := Tile{
		ID:     tree.NewNodeID2("\x00", 8),
		Leaves: []Node{{ID: tree.NewNodeID2("\x00\x05", 16)}},
	}
	for _, tc := range []struct {
		tile    Tile
		wantErr string
	}{
		{tile: Tile{ID: tree.NewNodeID2("\x01", 8)}},
		{tile: Tile{ID: tree.NewNodeID2("\x00", 8)}, wantErr: "exists"},
		{
			tile: Tile{
				ID:     tree.NewNodeID2("\x01", 8),
				Leaves: []Node{{ID: tree.NewNodeID2("\x00\x05\x00", 24)}},
			},
			wantErr: "invalid depth",
		},
	} {
		t.Run("", func(t *testing.T) {
			ts := NewTileSet(0, hasher, l)
			if err := ts.Add(existing); err != nil {
				t.Fatalf("Add: %v", err)
			}
			got := ""
			if err := ts.Add(tc.tile); err != nil {
				got = err.Error()
			}
			if want := tc.wantErr; len(want) == 0 && len(got) != 0 {
				t.Errorf("Add: %v", got)
			} else if len(want) != 0 && !strings.Contains(got, want) {
				t.Errorf("Add: err=%q; want err containing %q", got, want)
			}
		})
	}
}

func TestTileSetHashes(t *testing.T) {
	l := tree.NewLayout([]int{8, 8})
	ts := NewTileSet(0, maphasher.Default, l)

	count := 0
	for i, tc := range []struct {
		tile  Tile
		added int
	}{
		{tile: Tile{ID: tree.NewNodeID2("\x02", 8), Leaves: nil}, added: 0},
		{
			tile: Tile{
				ID:     tree.NewNodeID2("\x00", 8),
				Leaves: []Node{{ID: tree.NewNodeID2("\x00\x01", 16)}},
			},
			added: 8,
		},
		{
			tile: Tile{Leaves: []Node{
				{ID: tree.NewNodeID2("\x00", 8)},
				{ID: tree.NewNodeID2("\x02", 8)},
			}},
			added: 10,
		},
	} {
		if err := ts.Add(tc.tile); err != nil {
			t.Fatalf("Add(%d): %v", i, err)
		}
		count += tc.added
		// TODO(pavelkalinnikov): Check the nodes containment.
		if got, want := len(ts.Hashes()), count; got != want {
			t.Fatalf("Hashes(%d): got %d nodes, want %d", i, got, want)
		}
	}
}

func TestTileSetMutationBuild(t *testing.T) {
	l := tree.NewLayout([]int{8, 8})
	ids := []tree.NodeID2{
		tree.NewNodeID2("\x00\x00", 16),
		tree.NewNodeID2("\x00\x70", 16),
		tree.NewNodeID2("\x01\x01", 16),
		tree.NewNodeID2("\xFF\xFF", 16),
		tree.NewNodeID2("\x77\x77", 16),
		tree.NewNodeID2("\x77\x88", 16),
	}
	ts := NewTileSet(0, maphasher.Default, l)
	for _, tile := range []Tile{
		{Leaves: []Node{ // Root tile.
			{ID: ids[0].Prefix(8), Hash: []byte("hash_00")},
			{ID: ids[2].Prefix(8), Hash: []byte("hash_01")},
			{ID: ids[3].Prefix(8), Hash: []byte("hash_FF")},
		}},
		{ // Tile 0x00.
			ID: ids[0].Prefix(8),
			Leaves: []Node{
				{ID: ids[0], Hash: []byte("hash_0000")},
				{ID: ids[1], Hash: []byte("hash_0070")},
			},
		},
		{ // Tile 0x01.
			ID:     ids[2].Prefix(8),
			Leaves: []Node{{ID: ids[2], Hash: []byte("hash_0101")}},
		},
		{ // Tile 0xFF.
			ID:     ids[3].Prefix(8),
			Leaves: []Node{{ID: ids[3], Hash: []byte("hash_FFFF")}},
		},
	} {
		if err := ts.Add(tile); err != nil {
			t.Fatalf("Add(%v): %v", tile.ID, err)
		}
	}

	for _, tc := range []struct {
		upd  []Node                  // Node updates.
		want map[tree.NodeID2][]Node // Updated tiles.
	}{
		{upd: nil, want: make(map[tree.NodeID2][]Node)},
		{ // Updating the hash with the old value.
			upd:  []Node{{ID: ids[0], Hash: []byte("hash_0000")}},
			want: make(map[tree.NodeID2][]Node), // Tiles are intact.
		},
		{ // Ignoring non-leaf node updates for tiles.
			upd:  []Node{{ID: ids[0].Prefix(1), Hash: []byte("root")}},
			want: make(map[tree.NodeID2][]Node),
		},
		{ // Updating a non-existing node of a non-existing tile.
			upd: []Node{{ID: ids[4], Hash: []byte("new_7777")}},
			want: map[tree.NodeID2][]Node{
				ids[4].Prefix(8): {{ID: ids[4], Hash: []byte("new_7777")}},
			},
		},
		{ // Updating an existing node.
			upd: []Node{{ID: ids[0], Hash: []byte("new_0000")}},
			want: map[tree.NodeID2][]Node{
				ids[0].Prefix(8): {
					{ID: ids[0], Hash: []byte("new_0000")},
					{ID: ids[1], Hash: []byte("hash_0070")},
				},
			},
		},
		{ // Updating nodes of an existing tile.
			upd: []Node{{ID: ids[0].Sibling(), Hash: []byte("new_0001")}},
			want: map[tree.NodeID2][]Node{
				ids[0].Prefix(8): {
					{ID: ids[0], Hash: []byte("hash_0000")},
					{ID: ids[0].Sibling(), Hash: []byte("new_0001")},
					{ID: ids[1], Hash: []byte("hash_0070")},
				},
			},
		},
		{ // Multiple updates in order.
			upd: []Node{
				{ID: ids[0], Hash: []byte("new_0000")},
				{ID: ids[1], Hash: []byte("new_0001")},
			},
			want: map[tree.NodeID2][]Node{
				ids[0].Prefix(8): {
					{ID: ids[0], Hash: []byte("new_0000")},
					{ID: ids[1], Hash: []byte("new_0001")},
				},
			},
		},
		{ // Multiple updates out of order.
			upd: []Node{
				{ID: ids[1], Hash: []byte("new_0001")},
				{ID: ids[0], Hash: []byte("new_0000")},
			},
			want: map[tree.NodeID2][]Node{
				ids[0].Prefix(8): {
					{ID: ids[0], Hash: []byte("new_0000")},
					{ID: ids[1], Hash: []byte("new_0001")},
				},
			},
		},
		{ // Multiple updates of a non-existing tile in order.
			upd: []Node{
				{ID: ids[4], Hash: []byte("new_7777")},
				{ID: ids[5], Hash: []byte("new_7788")},
			},
			want: map[tree.NodeID2][]Node{
				ids[4].Prefix(8): {
					{ID: ids[4], Hash: []byte("new_7777")},
					{ID: ids[5], Hash: []byte("new_7788")},
				},
			},
		},
		{ // Multiple updates of a non-existing tile out of order.
			upd: []Node{
				{ID: ids[5], Hash: []byte("new_7788")},
				{ID: ids[4], Hash: []byte("new_7777")},
			},
			want: map[tree.NodeID2][]Node{
				ids[4].Prefix(8): {
					{ID: ids[4], Hash: []byte("new_7777")},
					{ID: ids[5], Hash: []byte("new_7788")},
				},
			},
		},
		{ // Updating multiple existing tiles.
			upd: []Node{
				{ID: ids[0], Hash: []byte("new_0000")},
				{ID: ids[2], Hash: []byte("new_0101")},
			},
			want: map[tree.NodeID2][]Node{
				ids[0].Prefix(8): {
					{ID: ids[0], Hash: []byte("new_0000")},
					{ID: ids[1], Hash: []byte("hash_0070")},
				},
				ids[2].Prefix(8): {
					{ID: ids[2], Hash: []byte("new_0101")},
				},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			m := NewTileSetMutation(ts)
			for _, node := range tc.upd {
				m.Set(node.ID, node.Hash)
			}
			tiles, err := m.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			got := make(map[tree.NodeID2][]Node, len(tiles))
			for _, tile := range tiles {
				got[tile.ID] = tile.Leaves
			}
			if want := tc.want; !reflect.DeepEqual(got, want) {
				t.Logf("%v", got)
				t.Logf("%v", want)
				t.Fatalf("Tile mismatch: got %d tiles, want %d other ones", len(got), len(want))
			}
		})
	}
}

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
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage/tree"
)

func TestTileMerge(t *testing.T) {
	ids := []tree.NodeID2{
		tree.NewNodeID2("\xAB\x00", 15),
		tree.NewNodeID2("\xAB\x10", 15),
		tree.NewNodeID2("\xAB\x20", 15),
		tree.NewNodeID2("\xAB\x30", 15),
		tree.NewNodeID2("\xAB\x40", 15),
	}
	id := ids[0].Prefix(8)
	n := func(idIndex int, hash string) Node {
		return Node{ID: ids[idIndex], Hash: []byte(hash)}
	}

	for _, tc := range []struct {
		desc string
		was  []Node
		upd  []Node
		want []Node
	}{
		{desc: "empty", want: []Node{}},
		{desc: "add-to-empty", upd: []Node{n(0, "h")}, want: []Node{n(0, "h")}},
		{
			desc: "override-one",
			was:  []Node{n(0, "old")},
			upd:  []Node{n(0, "new")},
			want: []Node{n(0, "new")},
		},
		{
			desc: "add-multiple",
			was:  []Node{n(0, "old0"), n(3, "old3")},
			upd:  []Node{n(1, "new1"), n(2, "new2"), n(4, "new4")},
			want: []Node{n(0, "old0"), n(1, "new1"), n(2, "new2"), n(3, "old3"), n(4, "new4")},
		},
		{
			desc: "override-some",
			was:  []Node{n(0, "old0"), n(1, "old1"), n(2, "old2"), n(3, "old3")},
			upd:  []Node{n(1, "new1"), n(2, "new2")},
			want: []Node{n(0, "old0"), n(1, "new1"), n(2, "new2"), n(3, "old3")},
		},
		{
			desc: "override-and-add",
			was:  []Node{n(0, "old0"), n(1, "old1"), n(3, "old3"), n(4, "old4")},
			upd:  []Node{n(1, "new1"), n(2, "new2")},
			want: []Node{n(0, "old0"), n(1, "new1"), n(2, "new2"), n(3, "old3"), n(4, "old4")},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			was := Tile{ID: id, Leaves: tc.was}
			upd := Tile{ID: id, Leaves: tc.upd}
			got, err := was.Merge(upd)
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			want := Tile{ID: id, Leaves: tc.want}
			if d := cmp.Diff(got, want, cmp.AllowUnexported(tree.NodeID2{})); d != "" {
				t.Errorf("Merge result mismatch:\n%s", d)
			}
		})
	}
}

func TestTileScan(t *testing.T) {
	lo := tree.NewLayout([]int{8, 24})
	h := bindHasher(maphasher.Default, 1)

	ids := []tree.NodeID2{
		tree.NewNodeID2("\x00", 8),
		tree.NewNodeID2("\x02", 8),
		tree.NewNodeID2("\xF0", 8),
		tree.NewNodeID2("\x00\x01\x02\x03", 32),
	}
	prefixes := func(high, low uint, ids ...tree.NodeID2) []tree.NodeID2 {
		ret := make([]tree.NodeID2, 0, int(high-low)*len(ids))
		for i := high; i > low; i-- {
			for _, id := range ids {
				ret = append(ret, id.Prefix(i))
			}
		}
		return ret
	}

	for _, tc := range []struct {
		id     tree.NodeID2
		leaves []Node
		visits []tree.NodeID2
	}{
		{leaves: nil, visits: []tree.NodeID2{}},
		{leaves: []Node{{ID: ids[0]}}, visits: prefixes(8, 0, ids[0])},
		{
			leaves: []Node{{ID: ids[0]}, {ID: ids[1]}},
			visits: append(prefixes(8, 6, ids[0], ids[1]), prefixes(6, 0, ids[0])...),
		},
		{
			leaves: []Node{{ID: ids[0]}, {ID: ids[2]}},
			visits: prefixes(8, 0, ids[0], ids[2]),
		},
		{id: ids[0], leaves: []Node{{ID: ids[3]}}, visits: prefixes(32, 8, ids[3])},
	} {
		t.Run("", func(t *testing.T) {
			tile := Tile{ID: tc.id, Leaves: tc.leaves}
			visits := make([]tree.NodeID2, 0, len(tc.visits))
			if err := tile.scan(lo, h, func(node Node) {
				visits = append(visits, node.ID)
			}); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if got, want := visits, tc.visits; !reflect.DeepEqual(got, want) {
				t.Errorf("visits mismatch: got %d nodes, want %d other ones", len(got), len(want))
			}
		})
	}
}

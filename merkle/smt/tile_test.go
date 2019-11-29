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

	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage/tree"
)

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

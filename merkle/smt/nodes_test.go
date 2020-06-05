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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/storage/tree"
)

func TestNewNodesRow(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		nodes   []Node
		want    NodesRow
		wantErr string
	}{
		{desc: "empty", want: nil},
		{
			desc:    "error-depth",
			nodes:   []Node{{ID: tree.NewNodeID2("00", 16)}, {ID: tree.NewNodeID2("001", 24)}},
			wantErr: "invalid depth",
		},
		{
			desc:    "error-dups",
			nodes:   []Node{{ID: tree.NewNodeID2("01", 16)}, {ID: tree.NewNodeID2("01", 16)}},
			wantErr: "duplicate ID",
		},
		{
			desc: "sorted",
			nodes: []Node{
				{ID: tree.NewNodeID2("01", 16)},
				{ID: tree.NewNodeID2("00", 16)},
				{ID: tree.NewNodeID2("02", 16)},
			},
			want: []Node{
				{ID: tree.NewNodeID2("00", 16)},
				{ID: tree.NewNodeID2("01", 16)},
				{ID: tree.NewNodeID2("02", 16)},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			row, err := NewNodesRow(tc.nodes)
			if err != nil {
				if tc.wantErr == "" {
					t.Fatalf("NewNodesRow: want no error, returned: %v", err)
				}
				if want := tc.wantErr; !strings.Contains(err.Error(), want) {
					t.Fatalf("NewNodesRow: got error: %v; want substring %q", err, want)
				}
			} else if want := tc.wantErr; want != "" {
				t.Fatalf("NewNodesRow: got no error; want prefix %q", want)
			}
			if d := cmp.Diff(row, tc.want, cmp.AllowUnexported(tree.NodeID2{})); d != "" {
				t.Errorf("NewNodesRow result mismatch:\n%s", d)
			}
		})
	}
}

func TestPrepare(t *testing.T) {
	id1 := tree.NewNodeID2("01234567890000000000000000000001", 256)
	id2 := tree.NewNodeID2("01234567890000000000000000000002", 256)
	id3 := tree.NewNodeID2("01234567890000000000000000000003", 256)
	id4 := tree.NewNodeID2("01234567890000000000000001111111", 256)

	for _, tc := range []struct {
		desc    string
		nodes   []Node
		want    []Node
		wantErr string
	}{
		{desc: "depth-err", nodes: []Node{{ID: id1.Prefix(10)}}, wantErr: "invalid depth"},
		{desc: "dup-err1", nodes: []Node{{ID: id1}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "dup-err2", nodes: []Node{{ID: id1}, {ID: id2}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "ok-empty", want: nil},
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
			err := Prepare(nodes, 256)
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

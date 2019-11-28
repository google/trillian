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

	"github.com/google/trillian/storage/tree"
)

func TestPrepare(t *testing.T) {
	id1 := tree.NewNodeID2("01234567890000000000000000000001", 256)
	id2 := tree.NewNodeID2("01234567890000000000000000000002", 256)
	id3 := tree.NewNodeID2("01234567890000000000000000000003", 256)
	id4 := tree.NewNodeID2("01234567890000000000000001111111", 256)

	for _, tc := range []struct {
		desc    string
		upd     []Node
		want    []Node
		wantErr string
	}{
		{desc: "depth-err", upd: []Node{{ID: id1.Prefix(10)}}, wantErr: "invalid depth"},
		{desc: "dup-err1", upd: []Node{{ID: id1}, {ID: id1}}, wantErr: "duplicate ID"},
		{desc: "dup-err2", upd: []Node{{ID: id1}, {ID: id2}, {ID: id1}}, wantErr: "duplicate ID"},
		{
			desc: "ok1",
			upd:  []Node{{ID: id2}, {ID: id1}, {ID: id4}, {ID: id3}},
			want: []Node{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
		{
			desc: "ok2",
			upd:  []Node{{ID: id4}, {ID: id3}, {ID: id2}, {ID: id1}},
			want: []Node{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			upd := tc.upd // No need to copy it here.
			err := Prepare(upd, 256)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if want := tc.wantErr; !strings.Contains(got, want) {
				t.Errorf("NewHStar3: want error containing %q, got %v", want, err)
			}
			if want := tc.want; want != nil && !reflect.DeepEqual(upd, want) {
				t.Errorf("NewHStar3: want updates:\n%v\ngot:\n%v", upd, want)
			}
		})
	}
}

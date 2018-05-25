// Copyright 2017 Google Inc. All Rights Reserved.
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

package election

import (
	"reflect"
	"testing"
)

type testOperation struct {
	id  int64
	val bool
}

func TestMasterTracker(t *testing.T) {
	var tests = []struct {
		ids   []int64
		ops   []testOperation
		count int
		held  []int64
		str   string
	}{
		{
			ids:   []int64{1, 2, 3},
			ops:   []testOperation{{1, true}},
			count: 1,
			held:  []int64{1},
			str:   "1 . .",
		},
		{
			ids:   []int64{1, 20000, 30000},
			ops:   []testOperation{{30000, true}},
			count: 1,
			held:  []int64{30000},
			str:   ". ..... 30000",
		},
		{
			ids: []int64{1, 2, 3},
			ops: []testOperation{
				{1, true},
				{2, true},
				{3, true},
				{1, false},
			},
			count: 2,
			held:  []int64{2, 3},
			str:   ". 2 3",
		},
		{
			ids: []int64{},
			ops: []testOperation{
				{1, true},
				{2, true},
				{3, true},
				{1, false},
			},
			count: 2,
			held:  []int64{2, 3},
			str:   ". 2 3",
		},
		{
			ids: []int64{1, 2, 3},
			ops: []testOperation{
				{1, true},
				{1, true}, // error: already true
			},
			count: 1, // count still accurate though
			held:  []int64{1},
			str:   "1 . .",
		},
		{
			ids: []int64{1, 2, 3},
			ops: []testOperation{
				{1, false}, // error: already false
			},
			count: 0, // count still accurate though
			held:  []int64{},
			str:   ". . .",
		},
	}

	for _, test := range tests {
		mt := NewMasterTracker(test.ids)
		for _, op := range test.ops {
			mt.Set(op.id, op.val)
		}
		if got := mt.Count(); got != test.count {
			t.Errorf("MasterTracker.Count(%+v)=%d; want %d", test.ops, got, test.count)
		}
		if got := mt.Held(); !reflect.DeepEqual(got, test.held) {
			t.Errorf("MasterTracker.Held(%+v)=%v; want %v", test.ops, got, test.held)
		}
		if got := mt.String(); got != test.str {
			t.Errorf("MasterTracker.String(%+v)=%q; want %q", test.ops, got, test.str)
		}
	}
}

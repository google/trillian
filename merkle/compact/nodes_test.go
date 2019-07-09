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

package compact

import (
	"fmt"
	"reflect"
	"testing"
)

func TestRangeNodesForPrefix(t *testing.T) {
	for _, tc := range []struct {
		size uint64
		want []NodeID
	}{
		{size: 0, want: []NodeID{}},
		{size: 1, want: []NodeID{{Level: 0, Index: 0}}},
		{size: 2, want: []NodeID{{Level: 1, Index: 0}}},
		{size: 3, want: []NodeID{{Level: 1, Index: 0}, {Level: 0, Index: 2}}},
		{size: 4, want: []NodeID{{Level: 2, Index: 0}}},
		{size: 5, want: []NodeID{{Level: 2, Index: 0}, {Level: 0, Index: 4}}},
		{size: 15, want: []NodeID{{Level: 3, Index: 0}, {Level: 2, Index: 2}, {Level: 1, Index: 6}, {Level: 0, Index: 14}}},
		{size: 100, want: []NodeID{{Level: 6, Index: 0}, {Level: 5, Index: 2}, {Level: 2, Index: 24}}},
		{size: 513, want: []NodeID{{Level: 9, Index: 0}, {Level: 0, Index: 512}}},
		{size: uint64(1) << 63, want: []NodeID{{Level: 63, Index: 0}}},
		{size: (uint64(1) << 63) + (uint64(1) << 57), want: []NodeID{{Level: 63, Index: 0}, {Level: 57, Index: 64}}},
	} {
		t.Run(fmt.Sprintf("size:%d", tc.size), func(t *testing.T) {
			if got, want := RangeNodesForPrefix(tc.size), tc.want; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RangeNodesForPrefix: got %v, want %v", got, want)
			}
		})
	}
}

func TestRangeNodes(t *testing.T) {
	n := func(level uint, index uint64) NodeID {
		return NewNodeID(level, index)
	}
	for _, tc := range []struct {
		begin uint64
		end   uint64
		want  []NodeID
	}{
		// Empty ranges.
		{end: 0, want: []NodeID{}},
		{begin: 10, end: 10, want: []NodeID{}},
		{begin: 1024, end: 1024, want: []NodeID{}},
		// One entry.
		{begin: 10, end: 11, want: []NodeID{n(0, 10)}},
		{begin: 1024, end: 1025, want: []NodeID{n(0, 1024)}},
		{begin: 1025, end: 1026, want: []NodeID{n(0, 1025)}},
		// Two entries.
		{begin: 10, end: 12, want: []NodeID{n(1, 5)}},
		{begin: 1024, end: 1026, want: []NodeID{n(1, 512)}},
		{begin: 1025, end: 1027, want: []NodeID{n(0, 1025), n(0, 1026)}},
		// Only left border.
		{begin: 0, end: 16, want: []NodeID{n(4, 0)}},
		{begin: 1, end: 16, want: []NodeID{n(0, 1), n(1, 1), n(2, 1), n(3, 1)}},
		{begin: 1, end: 16, want: []NodeID{n(0, 1), n(1, 1), n(2, 1), n(3, 1)}},
		{begin: 2, end: 16, want: []NodeID{n(1, 1), n(2, 1), n(3, 1)}},
		{begin: 3, end: 16, want: []NodeID{n(0, 3), n(2, 1), n(3, 1)}},
		{begin: 4, end: 16, want: []NodeID{n(2, 1), n(3, 1)}},
		{begin: 6, end: 16, want: []NodeID{n(1, 3), n(3, 1)}},
		{begin: 8, end: 16, want: []NodeID{n(3, 1)}},
		{begin: 11, end: 16, want: []NodeID{n(0, 11), n(2, 3)}},
		// Two-sided.
		{begin: 1, end: 31, want: []NodeID{n(0, 1), n(1, 1), n(2, 1), n(3, 1), n(3, 2), n(2, 6), n(1, 14), n(0, 30)}},
		{begin: 1, end: 17, want: []NodeID{n(0, 1), n(1, 1), n(2, 1), n(3, 1), n(0, 16)}},
	} {
		t.Run(fmt.Sprintf("range:%d:%d", tc.begin, tc.end), func(t *testing.T) {
			if got, want := RangeNodes(tc.begin, tc.end), tc.want; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RangeNodes: got %v, want %v", got, want)
			}
		})
	}
}

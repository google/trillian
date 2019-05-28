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

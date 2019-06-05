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

package cloudspanner

import (
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/compact"
)

func TestTreeOpts(t *testing.T) {
	opts := TreeOpts{ShardLevels: 2, LeafShards: 3}
	opts2 := TreeOpts{ShardLevels: 1, LeafShards: 2}
	for _, tc := range []struct {
		opts TreeOpts
		id   compact.NodeID
		want int32
	}{
		{opts: opts, id: compact.NewNodeID(0, 0), want: 0},
		{opts: opts, id: compact.NewNodeID(0, 1), want: 0},
		{opts: opts, id: compact.NewNodeID(0, 2), want: 1},
		{opts: opts, id: compact.NewNodeID(0, 3), want: 1},
		{opts: opts, id: compact.NewNodeID(0, 4), want: 2},
		{opts: opts, id: compact.NewNodeID(0, 5), want: 2},
		{opts: opts, id: compact.NewNodeID(0, 6), want: 0},
		{opts: opts, id: compact.NewNodeID(0, 7), want: 0},
		{opts: opts, id: compact.NewNodeID(0, 8), want: 1},
		{opts: opts, id: compact.NewNodeID(1, 0), want: 0},
		{opts: opts, id: compact.NewNodeID(1, 1), want: 1},
		{opts: opts, id: compact.NewNodeID(1, 2), want: 2},
		{opts: opts, id: compact.NewNodeID(1, 3), want: 0},
		{opts: opts, id: compact.NewNodeID(1, 4), want: 1},
		{opts: opts, id: compact.NewNodeID(2, 0), want: 3},
		{opts: opts2, id: compact.NewNodeID(0, 10), want: 0},
		{opts: opts2, id: compact.NewNodeID(0, 15), want: 1},
		{opts: opts2, id: compact.NewNodeID(10, 20), want: 2},
	} {
		t.Run(fmt.Sprintf("%+v:%+v", tc.opts, tc.id), func(t *testing.T) {
			if got, want := tc.opts.shardID(tc.id), tc.want; got != want {
				t.Fatalf("shardID=%d, want %d", got, want)
			}
		})
	}
}

func TestPackNodeID(t *testing.T) {
	for _, tc := range []struct {
		id   compact.NodeID
		want uint64
	}{
		{id: compact.NewNodeID(63, 0), want: 1},
		{id: compact.NewNodeID(62, 0), want: 2},
		{id: compact.NewNodeID(62, 1), want: 3},
		{id: compact.NewNodeID(60, 5), want: 13},
		{id: compact.NewNodeID(32, 0), want: 2147483648},
		{id: compact.NewNodeID(32, 1<<30), want: 3221225472},
		{id: compact.NewNodeID(0, 0), want: 9223372036854775808},
		{id: compact.NewNodeID(0, 1<<30), want: 9223372037928517632},
		{id: compact.NewNodeID(0, (1<<63)-1), want: 0xFFFFFFFFFFFFFFFF},
	} {
		t.Run(fmt.Sprintf("%+v", tc.id), func(t *testing.T) {
			if got, want := packNodeID(tc.id), tc.want; got != want {
				t.Fatalf("packNodeID: got %d, want %d", got, want)
			}
		})
	}
}

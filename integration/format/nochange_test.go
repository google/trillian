// Copyright 2017 Google LLC. All Rights Reserved.
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

// Package format contains an integration test which builds a log using an
// in-memory storage end-to-end, and makes sure the SubtreeProto storage format
// has no regressions.
package format

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/storage/storagepb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/testing/protocmp"
)

// TestDBFormatNoChange ensures that SubtreeProto tiles stored in the database
// do not change. This test compares against a previously saved output.
func TestDBFormatNoChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "dump_tree_output_96", size: 96},
		{name: "dump_tree_output_871", size: 871},
		{name: "dump_tree_output_1000", size: 1000},
		{name: "dump_tree_output_1024", size: 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(tc.size, 50, "Leaf %d")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			saved, err := ioutil.ReadFile("testdata/" + tc.name)
			if err != nil {
				t.Fatalf("ReadFile(%v): %v", tc.name, err)
			}
			got := parseTiles(t, out)
			want := parseTiles(t, string(saved))
			if d := cmp.Diff(want, got, protocmp.Transform()); d != "" {
				t.Errorf("Diff(-want,+got):\n%s", d)
			}
		})
	}
}

func parseTiles(t *testing.T, text string) []*storagepb.SubtreeProto {
	t.Helper()
	parts := strings.Split(text, "\n\n")
	tiles := make([]*storagepb.SubtreeProto, len(parts))
	for i, part := range parts {
		var tile storagepb.SubtreeProto
		if err := prototext.Unmarshal([]byte(part), &tile); err != nil {
			t.Fatalf("Failed to unmarshal part %d: %v", i, err)
		}
		tiles[i] = &tile
	}
	return tiles
}

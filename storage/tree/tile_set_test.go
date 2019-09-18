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

package tree

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/google/trillian/storage/storagepb"
)

func TestTileSetOperations(t *testing.T) {
	sp := &storagepb.SubtreeProto{Prefix: []byte("abc")}
	id := TileID{Root: NewNodeIDFromHash(sp.Prefix)}
	type op struct {
		op string
		ok bool
	}
	for i, tc := range [][]op{
		{{op: "touch", ok: true}, {op: "touch", ok: false}},
		{{op: "add", ok: true}, {op: "add", ok: false}},
		{{op: "add", ok: true}, {op: "touch", ok: false}},
		{{op: "touch", ok: true}, {op: "add", ok: true}, {op: "add", ok: false}},
		{{op: "touch", ok: true}, {op: "add", ok: true}, {op: "touch", ok: false}},
		{{op: "get", ok: false}, {op: "touch", ok: true}, {op: "get", ok: false}},
		{{op: "touch", ok: true}, {op: "get", ok: false}},
		{{op: "get", ok: false}, {op: "add", ok: true}, {op: "get", ok: true}},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			ts := NewTileSet()
			for i, op := range tc {
				switch op.op {
				case "touch":
					if got, want := ts.Touch(id), op.ok; got != want {
						t.Errorf("Touch at #%d: got %v, want %v", i, got, want)
					}
				case "add":
					err := ts.Add(sp)
					if got, want := err == nil, op.ok; got != want {
						t.Errorf("Add at #%d: got error %v, want %v", i, err, want)
					}
				case "get":
					tile := ts.Get(id)
					if got, want := tile != nil, op.ok; got != want {
						t.Errorf("Get at #%d: got tile %v, want %v", i, got, want)
					}
				}
			}
		})
	}
}

func TestTileSetAddErrors(t *testing.T) {
	// Note: This returns new instances each time, for better tests isolation.
	all := func() []*storagepb.SubtreeProto {
		return []*storagepb.SubtreeProto{
			nil,
			{Prefix: nil},
			{Prefix: []byte{}},
			{Prefix: []byte("ab")},
			{Prefix: []byte("ac")},
			{Prefix: []byte("bc")},
			{Prefix: []byte("ab")},
		}
	}
	for i, tc := range []struct {
		tiles       []*storagepb.SubtreeProto
		wantLastErr string
	}{
		{tiles: all()[:1], wantLastErr: "tile is nil"},
		{tiles: all()[1:3], wantLastErr: "tile already exists"},
		{tiles: all()[3:7], wantLastErr: "tile already exists"},
		{tiles: all()[2:3]}, // No error.
		{tiles: all()[3:6]},
		{tiles: all()[4:7]},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			ts := NewTileSet()
			for i, sp := range tc.tiles[:len(tc.tiles)-1] {
				if err := ts.Add(sp); err != nil {
					t.Fatalf("Add #%d: %v", i, err)
				}
			}
			lastErr := ""
			if err := ts.Add(tc.tiles[len(tc.tiles)-1]); err != nil {
				lastErr = err.Error()
			}
			if got, want := lastErr, tc.wantLastErr; got != want {
				t.Fatalf("Add: error=%q, want %q", got, want)
			}
		})
	}
}

func TestTileSetTypicalRun(t *testing.T) {
	ids := make([]TileID, 10)
	for i := range ids {
		bytes := []byte{0, 1, 0, byte(i)}
		ids[i] = TileID{Root: NewNodeIDFromHash(bytes)}
	}
	ts := NewTileSet()

	// Mark the "read set" as touched.
	for i, id := range ids {
		if !ts.Touch(id) {
			t.Errorf("Touch #%d returned false", i)
		}
	}
	// Some IDs might be in the set already.
	for i, id := range ids {
		if ts.Touch(id) {
			t.Errorf("re-Touch #%d returned true", i)
		}
	}

	// Pretend we deduplicated IDs based on Touch responses, read the tiles data
	// from somewhere, and now inserting them.
	for i, id := range ids {
		sp := &storagepb.SubtreeProto{Prefix: id.AsBytes(), Depth: 8}
		if err := ts.Add(sp); err != nil {
			t.Errorf("Add #%d: %v", i, err)
		}
	}

	// Now we should be able to read them all.
	for i, id := range ids {
		tile := ts.Get(id)
		if tile == nil {
			t.Errorf("Get #%d: tile missing", i)
		} else if got, want := tile.Prefix, id.AsBytes(); !bytes.Equal(got, want) {
			t.Errorf("Get #%d: got tile ID %x, want %x", i, got, want)
		}
	}
}

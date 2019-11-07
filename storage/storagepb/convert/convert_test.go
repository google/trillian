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

package convert

import (
	"reflect"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
)

func TestUnmarshal(t *testing.T) {
	type mappy map[string][]byte
	for _, tc := range []struct {
		sp      *storagepb.SubtreeProto
		wantErr string
	}{
		{sp: &storagepb.SubtreeProto{}, wantErr: "wrong depth"},
		{sp: &storagepb.SubtreeProto{Depth: -1}, wantErr: "wrong depth"},
		{sp: &storagepb.SubtreeProto{Depth: 8, Leaves: mappy{"huh?": nil}}, wantErr: "base64"},
		{sp: &storagepb.SubtreeProto{Depth: 8, Leaves: mappy{"": nil}}, wantErr: "ParseSuffix: empty bytes"},
		{sp: &storagepb.SubtreeProto{Depth: 12, Leaves: mappy{"DA==": nil}}, wantErr: "ParseSuffix: unexpected length"},
		{sp: &storagepb.SubtreeProto{Depth: 12, Leaves: mappy{"DAAA": nil}}},
		{sp: &storagepb.SubtreeProto{Depth: 13, Leaves: mappy{"DAAA": nil}}, wantErr: "wrong suffix bits"},
		{sp: &storagepb.SubtreeProto{Depth: 12, Leaves: mappy{"DAAA": nil, "DAAB": nil}}, wantErr: "Prepare"},
		{sp: &storagepb.SubtreeProto{Depth: 16, Leaves: mappy{"EAAA": nil, "EAAB": nil}}},
	} {
		t.Run("", func(t *testing.T) {
			got := ""
			if _, err := Unmarshal(tc.sp); err != nil {
				got = err.Error()
			}
			if want := tc.wantErr; len(want) == 0 && len(got) != 0 {
				t.Errorf("Unmarshal: %s, want no error", got)
			} else if !strings.Contains(got, want) {
				t.Errorf("Unmarshal: %s, want error containing %q", got, want)
			}
		})
	}
}

func TestMarshalErrors(t *testing.T) {
	for _, tc := range []struct {
		tile    smt.Tile
		height  uint
		wantErr string
	}{
		{tile: smt.Tile{}, height: 0, wantErr: "height out of"},
		{tile: smt.Tile{}, height: 256, wantErr: "height out of"},
		{tile: smt.Tile{ID: tree.NewNodeID2("\xFF", 5)}, height: 8, wantErr: "root unaligned"},
		{
			tile: smt.Tile{
				ID:     tree.NewNodeID2("\xFF", 8),
				Leaves: []smt.NodeUpdate{{ID: tree.NewNodeID2("\xFF0000", 24)}},
			},
			height:  8,
			wantErr: "wrong ID bits",
		},
		{
			tile: smt.Tile{
				ID:     tree.NewNodeID2("\xFF", 8),
				Leaves: []smt.NodeUpdate{{ID: tree.NewNodeID2("\xF000", 16)}},
			},
			height:  8,
			wantErr: "unrelated leaf ID",
		},
	} {
		t.Run("", func(t *testing.T) {
			_, err := Marshal(tc.tile, tc.height)
			if err == nil {
				t.Fatal("Marshal did not return error")
			}
			if got, want := err.Error(), tc.wantErr; !strings.Contains(got, want) {
				t.Errorf("Marshal: %s, want error containing %q", got, want)
			}
		})
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	upd := []smt.NodeUpdate{
		{ID: tree.NewNodeID2("0", 8), Hash: []byte("a")},
		{ID: tree.NewNodeID2("1", 8), Hash: []byte("b")},
	}
	deepUpd := []smt.NodeUpdate{
		{ID: tree.NewNodeID2("\x0F\x00\x00", 24), Hash: []byte("a")},
		{ID: tree.NewNodeID2("\x0F\xFF\x00", 24), Hash: []byte("b")},
		{ID: tree.NewNodeID2("\x0F\xFF\x01", 24), Hash: []byte("c")},
		{ID: tree.NewNodeID2("\x0F\xFF\x03", 24), Hash: []byte("d")},
		{ID: tree.NewNodeID2("\x0F\xFF\x09", 24), Hash: []byte("e")},
		{ID: tree.NewNodeID2("\x0F\xFF\xFF", 24), Hash: []byte("f")},
	}

	for _, tc := range []struct {
		tile   smt.Tile
		height uint
	}{
		{tile: smt.Tile{}, height: 8},
		{tile: smt.Tile{Leaves: upd[:1]}, height: 8},
		{tile: smt.Tile{Leaves: upd}, height: 8},
		{tile: smt.Tile{ID: tree.NewNodeID2("\x0F", 0), Leaves: deepUpd}, height: 24},
		{tile: smt.Tile{ID: tree.NewNodeID2("\x0F", 8), Leaves: deepUpd}, height: 16},
		{tile: smt.Tile{ID: tree.NewNodeID2("\x0F\xFF", 16), Leaves: deepUpd[1:]}, height: 8},
	} {
		t.Run("", func(t *testing.T) {
			sp, err := Marshal(tc.tile, tc.height)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			clone := proto.Clone(sp).(*storagepb.SubtreeProto) // Break memory dependency.
			tile, err := Unmarshal(clone)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got, want := tile, tc.tile; !reflect.DeepEqual(got, want) {
				t.Errorf("Tile mismatch: got %v, want %v", got, want)
			}
		})
	}
}

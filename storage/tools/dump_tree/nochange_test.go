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

package main

import (
	"io/ioutil"
	"strings"
	"testing"

	_ "github.com/google/trillian/crypto/keys/der/proto"
	"github.com/google/trillian/storage/storagepb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// TestDBFormatNoChange ensures that the prefix, suffix, and protos stored in the database do not change.
// This test compares the output from dump_tree against a previously saved output.
func TestDBFormatNoChange(t *testing.T) {
	for _, tc := range []struct {
		desc string
		file string
		opts Options
	}{
		{
			desc: "tree_size: 96",
			file: "testdata/dump_tree_output_96",
			opts: Options{
				96, 50,
				"Leaf %d",
				true, false, false, false, false, true, false, false,
			},
		},
		{
			desc: "tree_size: 871",
			file: "testdata/dump_tree_output_871",
			opts: Options{
				871, 50,
				"Leaf %d",
				true, false, false, false, false, true, false, false,
			},
		},
		{
			desc: "tree_size: 1000",
			file: "testdata/dump_tree_output_1000",
			opts: Options{
				1000, 50,
				"Leaf %d",
				true, false, false, false, false, true, false, false,
			},
		},
		{
			desc: "tree_size: 1024",
			file: "testdata/dump_tree_output_1024",
			opts: Options{
				1024, 50,
				"Leaf %d",
				true, false, false, false, false, true, false, false,
			},
		},
	} {
		out := Main(tc.opts)
		saved, err := ioutil.ReadFile(tc.file)
		if err != nil {
			t.Fatalf("ReadFile(%v): %v", tc.file, err)
		}

		savedS := strings.Split(string(saved), "\n\n")
		outS := strings.Split(out, "\n\n")
		if got, want := len(outS), len(savedS); got != want {
			t.Fatalf("%v dump_tree: got %v lines, want %v", tc.desc, got, want)
		}
		for i := range savedS {
			var got, want storagepb.SubtreeProto
			if err := prototext.Unmarshal([]byte(outS[i]), &got); err != nil {
				t.Fatalf("Failed to unmarshal 'got': %v", err)
			}
			if err := prototext.Unmarshal([]byte(savedS[i]), &want); err != nil {
				t.Fatalf("Failed to unmarshal 'want': %v", err)
			}
			if !proto.Equal(&got, &want) {
				t.Errorf("%v dump_tree tile %d:\n%v\nwant:\n%v", tc.desc, i, outS[i], savedS[i])
			}
		}
	}
}

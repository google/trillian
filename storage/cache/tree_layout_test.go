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

package cache

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/trillian/storage/tree"
	"github.com/kylelemons/godebug/pretty"
)

func TestSplitNodeID(t *testing.T) {
	layout := newTreeLayout(defaultMapStrata)
	for _, tc := range []struct {
		inPath        []byte
		inPathLenBits int
		outPrefix     []byte
		outSuffixBits int
		outSuffix     []byte
	}{
		{[]byte{0x12, 0x34, 0x56, 0x7f}, 32, []byte{0x12, 0x34, 0x56}, 8, []byte{0x7f}},
		{[]byte{0x12, 0x34, 0x56, 0xff}, 29, []byte{0x12, 0x34, 0x56}, 5, []byte{0xf8}},
		{[]byte{0x12, 0x34, 0x56, 0xff}, 25, []byte{0x12, 0x34, 0x56}, 1, []byte{0x80}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 16, []byte{0x12}, 8, []byte{0x34}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 9, []byte{0x12}, 1, []byte{0x00}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 8, []byte{}, 8, []byte{0x12}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 7, []byte{}, 7, []byte{0x12}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 0, []byte{}, 0, []byte{0}},
		{[]byte{0x70}, 2, []byte{}, 2, []byte{0x40}},
		{[]byte{0x70}, 3, []byte{}, 3, []byte{0x60}},
		{[]byte{0x70}, 4, []byte{}, 4, []byte{0x70}},
		{[]byte{0x70}, 5, []byte{}, 5, []byte{0x70}},
		{[]byte{0x00, 0x03}, 16, []byte{0x00}, 8, []byte{0x03}},
		{[]byte{0x00, 0x03}, 15, []byte{0x00}, 7, []byte{0x02}},
	} {
		n := tree.NewNodeIDFromHash(tc.inPath)
		n.PrefixLenBits = tc.inPathLenBits

		t.Run(fmt.Sprintf("%v", n), func(t *testing.T) {
			p, s := layout.split(n)
			if got, want := p.Root.Path, tc.outPrefix; !bytes.Equal(got, want) {
				t.Errorf("prefix %x, want %x", got, want)
			}
			if got, want := int(s.Bits()), tc.outSuffixBits; got != want {
				t.Errorf("suffix.Bits %v, want %v", got, want)
			}
			if got, want := s.Path(), tc.outSuffix; !bytes.Equal(got, want) {
				t.Errorf("suffix.Path %x, want %x", got, want)
			}
		})
	}
}

func TestStrataIndex(t *testing.T) {
	heights := []int{8, 8, 16, 32, 64, 128}
	want := []stratumInfo{{0, 8}, {1, 8}, {2, 16}, {2, 16}, {4, 32}, {4, 32}, {4, 32}, {4, 32}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}}

	layout := newTreeLayout(heights)
	if diff := pretty.Compare(layout.sIndex, want); diff != "" {
		t.Fatalf("sIndex diff:\n%v", diff)
	}
}

func TestDefaultMapStrataIndex(t *testing.T) {
	layout := newTreeLayout(defaultMapStrata)
	for _, tc := range []struct {
		depth int
		want  stratumInfo
	}{
		{0, stratumInfo{0, 8}},
		{1, stratumInfo{0, 8}},
		{7, stratumInfo{0, 8}},
		{8, stratumInfo{1, 8}},
		{15, stratumInfo{1, 8}},
		{79, stratumInfo{9, 8}},
		{80, stratumInfo{10, 176}},
		{81, stratumInfo{10, 176}},
		{156, stratumInfo{10, 176}},
	} {
		t.Run(fmt.Sprintf("depth:%d", tc.depth), func(t *testing.T) {
			got := layout.getStratumAt(tc.depth)
			if want := tc.want; got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}
		})
	}
}

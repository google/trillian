// Copyright 2019 Google LLC. All Rights Reserved.
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

	"github.com/google/trillian/merkle/compact"
)

func TestGetTileID(t *testing.T) {
	for _, tc := range []struct {
		id   compact.NodeID
		want []byte
	}{
		{id: nID(0, 0), want: []byte{0, 0, 0, 0, 0, 0, 0}},
		{id: nID(0, 255), want: []byte{0, 0, 0, 0, 0, 0, 0}},
		{id: nID(0, 256), want: []byte{0, 0, 0, 0, 0, 0, 1}},
		{id: nID(0, 12345), want: []byte{0, 0, 0, 0, 0, 0, 48}},
		{id: nID(3, 31), want: []byte{0, 0, 0, 0, 0, 0, 0}},
		{id: nID(3, 32), want: []byte{0, 0, 0, 0, 0, 0, 1}},
		{id: nID(7, 1), want: []byte{0, 0, 0, 0, 0, 0, 0}},
		{id: nID(7, 2), want: []byte{0, 0, 0, 0, 0, 0, 1}},
		{id: nID(8, 0), want: []byte{0, 0, 0, 0, 0, 0}},
		{id: nID(10, 129), want: []byte{0, 0, 0, 0, 0, 2}},
		{id: nID(20, 0x14B8DC5C), want: []byte{0x00, 0x01, 0x4B, 0x8D, 0xC5}},
		{id: nID(47, 0), want: []byte{0, 0}},
		{id: nID(47, 1), want: []byte{0, 0}},
		{id: nID(48, 1234), want: []byte{4}},
		{id: nID(60, 10), want: []byte{}},
		{id: nID(64, 0), want: []byte{}},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.id.Level, tc.id.Index), func(t *testing.T) {
			if got, want := getTileID(tc.id), tc.want; !bytes.Equal(got, want) {
				t.Errorf("getTileID: got %x, want %x", got, want)
			}
		})
	}
}

func TestSplitID(t *testing.T) {
	for _, tc := range []struct {
		id            compact.NodeID
		outPrefix     []byte
		outSuffixBits int
		outSuffix     []byte
	}{
		{nID(32, 0x1234567f), []byte{0x12, 0x34, 0x56}, 8, []byte{0x7f}},
		{nID(35, 0x123456ff>>3), []byte{0x12, 0x34, 0x56}, 5, []byte{0xf8}},
		{nID(39, 0x123456ff>>7), []byte{0x12, 0x34, 0x56}, 1, []byte{0x80}},
		{nID(48, 0x12345678>>16), []byte{0x12}, 8, []byte{0x34}},
		{nID(55, 0x12345678>>23), []byte{0x12}, 1, []byte{0x00}},
		{nID(56, 0x12345678>>24), []byte{}, 8, []byte{0x12}},
		{nID(57, 0x12345678>>25), []byte{}, 7, []byte{0x12}},
		{nID(64, 0x12345678>>32), []byte{}, 0, []byte{0}},
		{nID(62, 0x70>>6), []byte{}, 2, []byte{0x40}},
		{nID(61, 0x70>>5), []byte{}, 3, []byte{0x60}},
		{nID(60, 0x70>>4), []byte{}, 4, []byte{0x70}},
		{nID(59, 0x70>>3), []byte{}, 5, []byte{0x70}},
		{nID(48, 0x0003), []byte{0x00}, 8, []byte{0x03}},
		{nID(49, 0x0003>>1), []byte{0x00}, 7, []byte{0x02}},
	} {
		t.Run(fmt.Sprintf("%v", tc.id), func(t *testing.T) {
			p, s := splitID(tc.id)
			if got, want := p, tc.outPrefix; !bytes.Equal(got, want) {
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

func nID(level uint, index uint64) compact.NodeID {
	return compact.NewNodeID(level, index)
}

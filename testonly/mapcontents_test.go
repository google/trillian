// Copyright 2018 Google Inc. All Rights Reserved.
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

package testonly

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/maphasher"
)

func TestTruncateToNBits(t *testing.T) {
	tests := []struct {
		desc  string
		in    bitString
		inLen int
		want  bitString
	}{
		{
			desc:  "whole to whole bytes",
			in:    bitString{data: [32]byte{0x12, 0x34}, bitLen: 16},
			inLen: 8,
			want:  bitString{data: [32]byte{0x12}, bitLen: 8},
		},
		{
			desc:  "partial to whole bytes",
			in:    bitString{data: [32]byte{0x12, 0x34, 0x80}, bitLen: 17},
			inLen: 8,
			want:  bitString{data: [32]byte{0x12}, bitLen: 8},
		},
		{
			desc:  "whole to partial bytes",
			in:    bitString{data: [32]byte{0x12, 0x34}, bitLen: 16},
			inLen: 3,
			want:  bitString{data: [32]byte{0x00}, bitLen: 3},
		},
		{
			desc:  "to zero length",
			in:    bitString{data: [32]byte{0x12, 0x34}, bitLen: 16},
			inLen: 0,
			want:  bitString{data: [32]byte{}, bitLen: 0},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := truncateToNBits(test.in, test.inLen)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("truncateToNBits(%+v,%d)=%+v, want %+v", test.in, test.inLen, got, test.want)
			}
		})
	}
}

func TestExtendByABit(t *testing.T) {
	tests := []struct {
		desc string
		in   bitString
		val  int
		want bitString
	}{
		{
			desc: "whole bytes with zero",
			in:   bitString{data: [32]byte{0x12, 0x34}, bitLen: 16},
			val:  0,
			want: bitString{data: [32]byte{0x12, 0x34, 0x00}, bitLen: 17},
		},
		{
			desc: "whole bytes with one",
			in:   bitString{data: [32]byte{0x12, 0x34}, bitLen: 16},
			val:  1,
			want: bitString{data: [32]byte{0x12, 0x34, 0x80}, bitLen: 17},
		},
		{
			desc: "partial bytes with zero",
			in:   bitString{data: [32]byte{0x12, 0x34, 0x80}, bitLen: 18},
			val:  0,
			want: bitString{data: [32]byte{0x12, 0x34, 0x80}, bitLen: 19},
		},
		{
			desc: "partial bytes with one",
			in:   bitString{data: [32]byte{0x12, 0x34, 0x80}, bitLen: 18},
			val:  1,
			want: bitString{data: [32]byte{0x12, 0x34, 0xa0}, bitLen: 19},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := extendByABit(test.in, test.val)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("extendByABit(%+v,%d)=%+v, want %+v", test.in, test.val, got, test.want)
			}
		})
	}
}

var (
	key0 = [32]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	value0 = "value-0000"
)

func TestRootHash(t *testing.T) {
	hasher := maphasher.New(crypto.SHA256)
	t.Logf("Using map hasher %s", hasher)
	hasher.HashChildren([]byte{0x01}, []byte{0x02})
	tests := []struct {
		desc     string
		contents *MapContents
		want     string // hex-encoded
	}{
		{
			desc: "nil",
			want: "c6689f10812a0980976d9533d83875282166159567ec35155716c1413af53d6a",
		},
		{
			desc:     "empty",
			contents: &MapContents{},
			want:     "c6689f10812a0980976d9533d83875282166159567ec35155716c1413af53d6a",
		},
		{
			desc: "single-leaf",
			contents: &MapContents{
				Rev:  0,
				data: map[mapKey]string{key0: value0},
			},
			want: "e05d802c7900e7d1f8de7a400a7873eee947724b38eaaa03438f6fc5fcbe39d4",
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := test.contents.RootHash(0, hasher)
			if err != nil {
				t.Fatalf("RootHash()=nil,%v, want _,nil", err)
			}
			want, _ := hex.DecodeString(test.want)
			if !bytes.Equal(got, want) {
				t.Fatalf("RootHash()=%x, want %x", got, want)
			}
		})
	}
}

func TestVersionedMapContents_UpdateContents(t *testing.T) {
	type update struct {
		rev    uint64
		leaves []*trillian.MapLeaf
	}
	tests := []struct {
		desc    string
		updates []update
		wantErr bool
	}{
		{
			desc: "single revision",
			updates: []update{
				{
					rev:    1,
					leaves: []*trillian.MapLeaf{},
				},
			},
		}, {
			desc: "duplicate revision is error",
			updates: []update{
				{
					rev:    1,
					leaves: []*trillian.MapLeaf{},
				}, {
					rev:    1,
					leaves: []*trillian.MapLeaf{},
				},
			},
			wantErr: true,
		}, {
			desc: "unordered revisions is error",
			updates: []update{
				{
					rev:    2,
					leaves: []*trillian.MapLeaf{},
				}, {
					rev:    1,
					leaves: []*trillian.MapLeaf{},
				},
			},
			wantErr: true,
		}, {
			desc: "consecutive revision",
			updates: []update{
				{
					rev:    1,
					leaves: []*trillian.MapLeaf{},
				}, {
					rev:    2,
					leaves: []*trillian.MapLeaf{},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var vmc VersionedMapContents
			var gotErr error
			for _, u := range test.updates {
				if _, e := vmc.UpdateContentsWith(u.rev, u.leaves); e != nil {
					gotErr = e
				}
			}
			if (gotErr != nil) != test.wantErr {
				t.Errorf("Unexpected error state: %v, wantErr: %v", gotErr, test.wantErr)
			}
		})
	}
}

func TestPickRevision(t *testing.T) {
	var vmc VersionedMapContents
	for rev := 1; rev < 5; rev++ {
		_, err := vmc.UpdateContentsWith(uint64(rev), []*trillian.MapLeaf{})
		if err != nil {
			t.Fatalf("Error adding revision %d", rev)
		}
	}

	for want := 1; want < 5; want++ {
		if got := vmc.PickRevision(uint64(want)).Rev; got != int64(want) {
			t.Fatalf("PickRevision()=%x, want %x", got, want)
		}
	}

	if got := vmc.PickRevision(0); got != nil {
		t.Fatalf("PickRevision(0) should be nil, was %v", got)
	}
	if got := vmc.PickRevision(5); got != nil {
		t.Fatalf("PickRevision(5) should be nil, was %v", got)
	}
}

// Copyright 2017 Google Inc. All Rights Reserved.
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

package storage

import (
	"bytes"
	"encoding/base64"
	"math/big"
	"testing"
)

const (
	logStrataDepth = 8
	maxLogDepth    = 64
)

func TestSuffixKeyEquals(t *testing.T) {
	for _, tc := range []struct {
		prefix    []byte
		leafIndex int64
		want      []byte
	}{
		{h2b(""), 1, h2b("0801")},
		{h2b("00"), 1, h2b("0801")},
	} {
		sfxA, err := makeSuffixKey(logStrataDepth, tc.leafIndex)
		if err != nil {
			t.Errorf("makeSuffixKey(%v, %v): %v", logStrataDepth, tc.leafIndex, err)
			continue
		}

		sfxABytes, err := base64.StdEncoding.DecodeString(sfxA)
		if err != nil {
			t.Errorf("makeSuffixKey(%v, %v): %v", logStrataDepth, tc.leafIndex, err)
			continue
		}

		if got, want := sfxABytes, tc.want; !bytes.Equal(got, want) {
			t.Errorf("makeSuffixKey(%v, %v): %x, want %x", logStrataDepth, tc.leafIndex, got, want)
			continue
		}

		nodeID := NewNodeIDFromPrefix(tc.prefix, logStrataDepth, tc.leafIndex, logStrataDepth, maxLogDepth)
		_, sfxB := nodeID.Split(len(tc.prefix), logStrataDepth)
		sfxBKey := sfxB.Serialize()
		sfxBBytes, err := base64.StdEncoding.DecodeString(sfxBKey)
		if err != nil {
			t.Errorf("splitNodeID(%v): _, %v", nodeID, err)
			continue
		}

		if got, want := sfxBBytes, tc.want; !bytes.Equal(got, want) {
			t.Errorf("[%x, %v].splitNodeID(%v, %v): %v.Serialize(): %x, want %x", nodeID.Path, nodeID.PrefixLenBits, len(tc.prefix), logStrataDepth, sfxB, got, want)
			continue
		}
	}
}

func TestSuffixKey(t *testing.T) {
	for _, tc := range []struct {
		depth   int
		index   int64
		want    []byte
		wantErr bool
	}{
		{depth: 0, index: 0x00, want: h2b("0000"), wantErr: false},
		{depth: 8, index: 0x00, want: h2b("0800"), wantErr: false},
		// TODO(gdbelvin): want: "0fabcd"?
		{depth: 8, index: 0xabcd, want: h2b("08cd"), wantErr: false},
		// TODO(gdbelvin): want "0240"
		{
			depth:   2,
			index:   new(big.Int).SetBytes(h2b("4000000000000000000000000000000000000000000000000000000000000000")).Int64(),
			want:    h2b("0200"),
			wantErr: false,
		},
		{depth: 15, index: 0xab, want: h2b("0fab"), wantErr: false},
		{depth: 16, index: 0x00, want: h2b("1000"), wantErr: false},
	} {
		suffixKey, err := makeSuffixKey(tc.depth, tc.index)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("makeSuffixKey(%v, %v): %v, want err: %v",
				tc.depth, tc.index, err, want)
			continue
		}
		if err != nil {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(suffixKey)
		if err != nil {
			t.Errorf("DecodeString(%v): %v", suffixKey, err)
			continue
		}
		if got, want := b, tc.want; !bytes.Equal(got, want) {
			t.Errorf("makeSuffixKey(%v, %x): %x, want %x",
				tc.depth, tc.index, got, want)
		}
	}
}

func TestSuffixSerialize(t *testing.T) {
	for _, tc := range []struct {
		s    Suffix
		want string
	}{
		// Prexisting format. This test vector must NOT change or existing data will be inaccessible.
		{s: Suffix{5, []byte{0xae}}, want: "Ba4="},
	} {
		if got, want := tc.s.Serialize(), tc.want; got != want {
			t.Errorf("%v.serialize(): %v, want %v", tc.s, got, want)
		}
	}
}

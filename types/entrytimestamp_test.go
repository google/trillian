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

package types

import (
	"bytes"
	"encoding"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"

	timestamppb "github.com/golang/protobuf/ptypes/timestamp"
)

var dehex = testonly.MustHexDecode

func TestEntryTimestampRoundTrip(t *testing.T) {
	// *EntryTimestampV1 should satisfy both the encoding.BinaryMarshaler
	// and encoding.BinaryUnmarshaler interfaces.
	type marshmallow interface {
		encoding.BinaryMarshaler
		encoding.BinaryUnmarshaler
	}
	ets := []marshmallow{
		&EntryTimestampV1{TimestampNanos: 1000000, LeafIdentityHash: []byte{0x01, 0x02}, MerkleLeafHash: []byte{0x02, 0x03}},
		&EntryTimestampV1{TimestampNanos: 0, LeafIdentityHash: []byte{}, MerkleLeafHash: []byte{}},
	}

	for _, et := range ets {
		b, err := et.MarshalBinary()
		if err != nil {
			t.Errorf("%v MarshalBinary()=_,%v; want _,nil", et, err)
			continue
		}
		var got EntryTimestampV1
		if err := got.UnmarshalBinary(b); err != nil {
			t.Errorf("UnmarshalBinary()=%v; want nil", err)
			continue
		}
		if !reflect.DeepEqual(&got, et) {
			t.Errorf("serialize/parse round trip failed. got %#v, want %#v", got, et)
		}
	}
}

func TestUnmarshalEntryTimestamp(t *testing.T) {
	var tests = []struct {
		desc    string
		in      []byte
		want    EntryTimestampV1
		wantErr string
	}{
		{
			desc: "EmptyEntry",
			in:   dehex("0001" + "0000000000000100" + "0000" + "0000"),
			want: EntryTimestampV1{TimestampNanos: 0x100, LeafIdentityHash: []byte{}, MerkleLeafHash: []byte{}},
		},
		{
			desc: "ValidEntry",
			in:   dehex("0001" + "0000000000000100" + "0002" + "1011" + "0003" + "010305"),
			want: EntryTimestampV1{TimestampNanos: 0x100, LeafIdentityHash: []byte{0x10, 0x11}, MerkleLeafHash: []byte{0x01, 0x03, 0x05}},
		},
		{
			desc:    "WrongVersion",
			in:      dehex("0000" + "0000000000000100" + "0000" + "0000"),
			wantErr: "invalid EntryTimestamp.Version",
		},
		{
			desc:    "TrailingData",
			in:      dehex("0001" + "0000000000000100" + "0000" + "0000" + "AA"),
			wantErr: "trailing data",
		},
		{
			desc:    "IncompleteData",
			in:      dehex("0001" + "0000"),
			wantErr: "syntax error",
		},
		{
			desc:    "NilInput",
			in:      nil,
			wantErr: "nil entry timestamp",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var got EntryTimestampV1
			err := got.UnmarshalBinary(test.in)
			if err != nil {
				if test.wantErr == "" {
					t.Fatalf("UnmarshalBinary(%x)=nil,%q; want _,nil", test.in, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("UnmarshalBinary(%x)=nil,%q; want nil, err containing %q", test.in, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("UnmarshalBinary(%x)=%+v,nil; want nil, err containing %q", test.in, got, test.wantErr)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("UnmarshalBinary(%x)=%+v; want %+v", test.in, got, test.want)
			}
		})
	}
}

func TestEntryDataForLeaf(t *testing.T) {
	var tests = []struct {
		desc    string
		in      trillian.LogLeaf
		want    []byte
		wantErr string
	}{
		{
			desc: "EmptyLeaf",
			in: trillian.LogLeaf{
				QueueTimestamp: &timestamppb.Timestamp{},
			},
			want: dehex("0001" + "0000000000000000" + "0000" + "0000"),
		},
		{
			desc: "ValidEntry",
			in: trillian.LogLeaf{
				QueueTimestamp:   &timestamppb.Timestamp{Nanos: 0x100},
				LeafIdentityHash: []byte{0x10, 0x11},
				MerkleLeafHash:   []byte{0x01, 0x03, 0x05},
			},
			want: dehex("0001" + "0000000000000100" + "0002" + "1011" + "0003" + "010305"),
		},
		{
			desc: "InvalidTimestamp",
			in: trillian.LogLeaf{
				QueueTimestamp:   &timestamppb.Timestamp{Seconds: 1, Nanos: -1},
				LeafIdentityHash: []byte{0x10, 0x11},
				MerkleLeafHash:   []byte{0x01, 0x03, 0x05},
			},
			wantErr: "failed to parse timestamp",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := EntryDataForLeaf(&test.in)
			if err != nil {
				if test.wantErr == "" {
					t.Fatalf("EntryDataForLeaf(%+v)=nil,%q; want _,nil", test.in, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("EntryDataForLeaf(%+v)=nil,%q; want nil, err containing %q", test.in, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("EntryDataForLeaf(%+v)=%x,nil; want nil, err containing %q", test.in, got, test.wantErr)
			}
			if !bytes.Equal(got, test.want) {
				t.Errorf("EntryDataForLeaf(%+v)=%x; want %x", test.in, got, test.want)
			}
		})
	}
}

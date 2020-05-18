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

package hammer

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
)

var dehex = testonly.MustHexDecode

func TestWriteReadMessage(t *testing.T) {
	var tests = []struct {
		desc string
		in   proto.Message
		want string
	}{
		{
			desc: "get-smr-req",
			in:   &trillian.GetSignedMapRootRequest{MapId: 123},
			want: "type.googleapis.com/trillian.GetSignedMapRootRequest",
		},
		{
			desc: "get-tree",
			in:   &trillian.Tree{TreeId: 123},
			want: "type.googleapis.com/trillian.Tree",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var buf bytes.Buffer
			writer := bufio.NewWriter(&buf)
			err := writeMessage(writer, test.in)
			if err != nil {
				t.Fatalf("writeMessage(%T)=%v; want nil", test.in, err)
			}
			if err := writer.Flush(); err != nil {
				t.Errorf("Flush()=%v", err)
			}
			data := buf.Bytes()
			t.Logf("%+v => %x", test.in, data)

			readFrom := bytes.NewBuffer(data)
			got, err := readMessage(readFrom)
			if err != nil {
				t.Fatalf("readMessage(%x)=nil,%v; want _,nil", data, err)
			}
			if got.TypeUrl != test.want {
				t.Errorf("readMessage(%x)=%q; want %q", data, got.TypeUrl, test.want)
			}
		})
	}
}

func TestReadMessage(t *testing.T) {
	var tests = []struct {
		desc    string
		in      []byte
		want    string
		wantErr string
	}{
		{
			desc: "get-tree",
			in:   dehex("00000027" + "0a21747970652e676f6f676c65617069732e636f6d2f7472696c6c69616e2e547265651202087b"),
			want: "type.googleapis.com/trillian.Tree",
		},
		{
			desc:    "len-too-short",
			in:      dehex("0000"),
			wantErr: "expected 4-byte length",
		},
		{
			desc:    "data-too-short",
			in:      dehex("00000027" + "0a21747970652e676f6f676c65617069732e636f6d2f7472696c6c69616e2e54726565120208"),
			wantErr: "expected 38 bytes of data",
		},
		{
			desc:    "empty",
			in:      dehex(""),
			wantErr: "EOF",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			readFrom := bytes.NewBuffer(test.in)
			got, err := readMessage(readFrom)
			if err != nil {
				if test.wantErr == "" {
					t.Fatalf("readMessage(%x)=nil,%v; want _,nil", test.in, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("readMessage(%x)=nil,%v; want error containing %q", test.in, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("readMessage(%x)=%+v,nil; want nil,error containing %q", test.in, got, test.wantErr)
			}
			if got.TypeUrl != test.want {
				t.Errorf("readMessage(%x).TypeUrl=%q; want %q", test.in, got.TypeUrl, test.want)
			}
		})
	}
}

func TestConvertMsg(t *testing.T) {
	mapmap := map[int64]int64{999: 123}
	var tests = []struct {
		desc string
		in   proto.Message
		want proto.Message
	}{
		{
			desc: "get-smr-req-mapped",
			in:   &trillian.GetSignedMapRootRequest{MapId: 999},
			want: &trillian.GetSignedMapRootRequest{MapId: 123},
		},
		{
			desc: "get-smr-req-unmapped",
			in:   &trillian.GetSignedMapRootRequest{MapId: 456},
			want: &trillian.GetSignedMapRootRequest{MapId: 456},
		},
		{
			desc: "tree-id-instead",
			in:   &trillian.Tree{TreeId: 999},
			want: &trillian.Tree{TreeId: 999},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := test.in
			convertMessage(got, mapmap)
			if !proto.Equal(got, test.want) {
				t.Fatalf("convertMsg(%+v)=%+v; want %+v", test.in, got, test.want)
			}
		})
	}
}

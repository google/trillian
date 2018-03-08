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
	"reflect"
	"testing"

	_ "github.com/golang/glog" // Don't crash when --logtostderr is supplied
)

func TestMapRoot(t *testing.T) {
	for _, tc := range []struct {
		mapRoot *MapRootV1
	}{
		{mapRoot: &MapRootV1{
			RootHash: []byte("foo"),
			Metadata: []byte{},
		}},
	} {
		b, err := SerializeMapRoot(tc.mapRoot)
		if err != nil {
			t.Errorf("SerializeMapRoot(%v): %v", tc.mapRoot, err)
		}
		got, err := ParseMapRoot(b)
		if err != nil {
			t.Errorf("ParseMapRoot(): %v", err)
		}
		if !reflect.DeepEqual(got, tc.mapRoot) {
			t.Errorf("serialize/parse round trip failed. got %#v, want %#v", got, tc.mapRoot)
		}
	}
}

func MustSerializeMapRoot(root *MapRootV1) []byte {
	b, err := SerializeMapRoot(root)
	if err != nil {
		panic(err)
	}
	return b
}

func TestParseMapRoot(t *testing.T) {
	for _, tc := range []struct {
		mapRoot []byte
		want    *MapRootV1
		wantErr bool
	}{
		{
			want: &MapRootV1{
				RootHash: []byte("foo"),
				Metadata: []byte{},
			},
			mapRoot: MustSerializeMapRoot(&MapRootV1{
				RootHash: []byte("foo"),
				Metadata: []byte{},
			}),
		},
		{
			// Correct type, but junk afterwards.
			mapRoot: []byte{0, 1, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
			wantErr: true,
		},
		{mapRoot: []byte("foo"), wantErr: true},
		{mapRoot: nil, wantErr: true},
	} {
		r, err := ParseMapRoot(tc.mapRoot)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("ParseMapRoot(): %v, wantErr: %v", err, want)
		}
		if got, want := r, tc.want; !reflect.DeepEqual(got, want) {
			t.Errorf("ParseMapRoot(): %v, want: %v", got, want)
		}
	}
}

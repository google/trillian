// Copyright 2018 Google LLC. All Rights Reserved.
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
	"encoding"
	"reflect"
	"testing"

	_ "github.com/golang/glog" // Don't crash when --logtostderr is supplied
)

func TestMapRoot(t *testing.T) {
	for _, mapRoot := range []interface {
		encoding.BinaryMarshaler
		encoding.BinaryUnmarshaler
	}{
		&MapRootV1{
			RootHash: []byte("foo"),
			Metadata: []byte{},
		},
	} {
		b, err := mapRoot.MarshalBinary()
		if err != nil {
			t.Errorf("%v MarshalBinary(): %v", mapRoot, err)
		}
		var got MapRootV1
		if err := got.UnmarshalBinary(b); err != nil {
			t.Errorf("UnmarshalBinary(): %v", err)
		}
		if !reflect.DeepEqual(&got, mapRoot) {
			t.Errorf("serialize/parse round trip failed. got %#v, want %#v", got, mapRoot)
		}
	}
}

func MustMarshalMapRoot(root *MapRootV1) []byte {
	b, err := root.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return b
}

func TestUnmarshalMapRoot(t *testing.T) {
	for _, tc := range []struct {
		mapRoot []byte
		want    MapRootV1
		wantErr bool
	}{
		{
			mapRoot: MustMarshalMapRoot(&MapRootV1{
				RootHash: []byte("aaa"),
			}),
			want: MapRootV1{
				RootHash: []byte("aaa"),
				Metadata: []byte{},
			},
		},
		{
			// Correct type, but junk afterwards.
			mapRoot: []byte{0, 1, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
			wantErr: true,
		},
		{
			// Incorrect type
			mapRoot: []byte{0},
			wantErr: true,
		},
		{mapRoot: []byte("foo"), wantErr: true},
		{mapRoot: nil, wantErr: true},
	} {
		var r MapRootV1
		err := r.UnmarshalBinary(tc.mapRoot)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("UnmarshalBinary(): %v, wantErr: %v", err, want)
		}
		if got, want := r, tc.want; !reflect.DeepEqual(got, want) {
			t.Errorf("UnmarshalBinary(): \n%#v, want: \n%#v", got, want)
		}
	}
	// Unmarshaling to a nil should throw an error.
	var nilPtr *MapRootV1
	if err := nilPtr.UnmarshalBinary(MustMarshalMapRoot(&MapRootV1{})); err == nil {
		t.Errorf("nil.UnmarshalBinary(): %v, want err", err)
	}
}

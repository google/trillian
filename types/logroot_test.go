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

package types

import (
	"encoding"
	"reflect"
	"testing"
)

func TestLogRoot(t *testing.T) {
	for _, logRoot := range []interface {
		encoding.BinaryMarshaler
		encoding.BinaryUnmarshaler
	}{
		&LogRootV1{
			RootHash: []byte("foo"),
			Metadata: []byte{},
		},
	} {
		b, err := logRoot.MarshalBinary()
		if err != nil {
			t.Errorf("%v MarshalBinary(): %v", logRoot, err)
			continue
		}
		var got LogRootV1
		if err := got.UnmarshalBinary(b); err != nil {
			t.Errorf("UnmarshalBinary(): %v", err)
			continue
		}
		if !reflect.DeepEqual(&got, logRoot) {
			t.Errorf("serialize/parse round trip failed. got %#v, want %#v", got, logRoot)
		}
	}
}

func TestUnmarshalLogRoot(t *testing.T) {
	for _, tc := range []struct {
		logRoot []byte
		wantErr bool
	}{
		{logRoot: MustMarshalLogRoot(&LogRootV1{})},
		{
			logRoot: func() []byte {
				b := MustMarshalLogRoot(&LogRootV1{})
				b[0] = 1 // Corrupt the version tag.
				return b
			}(),
			wantErr: true,
		},
		{
			// Correct type, but junk afterwards.
			logRoot: []byte{0, 1, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
			wantErr: true,
		},
		{
			// Incorrect type.
			logRoot: []byte{0},
			wantErr: true,
		},
		{logRoot: []byte("foo"), wantErr: true},
		{logRoot: nil, wantErr: true},
	} {

		var got LogRootV1
		err := got.UnmarshalBinary(tc.logRoot)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("UnmarshalBinary(): %v, wantErr %v", err, want)
		}
	}

	// Unmarshaling to a nil should throw an error.
	var nilPtr *LogRootV1
	if err := nilPtr.UnmarshalBinary(MustMarshalLogRoot(&LogRootV1{})); err == nil {
		t.Errorf("nil.UnmarshalBinary(): %v, want err", err)
	}
}

func MustMarshalLogRoot(root *LogRootV1) []byte {
	b, err := root.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return b
}

func TestKeyHint(t *testing.T) {
	for _, tc := range []struct {
		hint    []byte
		want    int64
		wantErr bool
	}{
		{hint: SerializeKeyHint(4), want: 4},
		{hint: SerializeKeyHint(3561657513447883733), want: 3561657513447883733},
		{hint: []byte{0, 0, 0, 0, 0, 0, 0, 4}, want: 4},
		{hint: []byte{0xff, 0, 0, 0, 0, 0, 4}, want: 0, wantErr: true},    // Integer overflow
		{hint: []byte{0, 0, 0, 0, 0, 0, 0, 4, 0}, want: 0, wantErr: true}, // Wrong byte len
	} {
		logID, err := ParseKeyHint(tc.hint)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("ParseKeyHint(%v): %v, wantErr: %v", tc.hint, err, want)
		}
		if got, want := logID, tc.want; got != want {
			t.Errorf("ParseKeyHint(%v): %v, want: %v", tc.hint, got, want)
		}
	}
}

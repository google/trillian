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
	"reflect"
	"testing"
)

func TestLogRoot(t *testing.T) {
	for _, logRoot := range []*LogRootV1{
		{
			RootHash: []byte("foo"),
			Metadata: []byte{},
		},
	} {
		b, err := SerializeLogRoot(logRoot)
		if err != nil {
			t.Errorf("SerializeLogRoot(%v): %v", logRoot, err)
		}
		got, err := ParseLogRoot(b)
		if err != nil {
			t.Errorf("ParseLogRoot(): %v", err)
		}
		if !reflect.DeepEqual(got, logRoot) {
			t.Errorf("serialize/parse round trip failed. got %#v, want %#v", got, logRoot)
		}
	}
}

func TestParseLogRoot(t *testing.T) {
	for _, tc := range []struct {
		logRoot []byte
		wantErr bool
	}{
		{
			logRoot: func() []byte {
				b, _ := SerializeLogRoot(&LogRootV1{})
				return b
			}(),
		},
		{
			logRoot: func() []byte {
				b, _ := SerializeLogRoot(&LogRootV1{})
				b[0] = 1 // Corrupt the version tag.
				return b
			}(),
			wantErr: true,
		},
		{
			logRoot: []byte("foo"),
			wantErr: true,
		},
	} {
		_, err := ParseLogRoot(tc.logRoot)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("ParseLogRoot(): %v, wantErr: %v", err, want)
		}
	}
}

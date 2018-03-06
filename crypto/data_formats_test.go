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

package crypto

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
)

var dh = testonly.MustHexDecode

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

// It's important that signatures don't change.
func TestHashLogRootKnownValue(t *testing.T) {
	expectedSig := dh("5e6baba8dc3465de9c01d669059dda590b7ce123d6ccd436bcd898f1c79ff6d9")
	root := trillian.SignedLogRoot{
		TimestampNanos: 226770903,
		RootHash:       []byte("Some bytes that won't change"),
		TreeSize:       167329345,
	}
	hash, err := hashLogRoot(root)
	if err != nil {
		t.Fatalf("HashLogRoot(): %v", err)
	}
	if got, want := hash, expectedSig; !bytes.Equal(got, want) {
		t.Fatalf("TestHashLogRootKnownValue: got:%x, want:%x", got, want)
	}
}

func TestHashLogRoot(t *testing.T) {
	unique := make(map[[20]byte]bool)
	for _, test := range []struct {
		root trillian.SignedLogRoot
	}{
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Islington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267708,
				RootHash:       []byte("Islington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Oslington"),
				TreeSize:       2,
			},
		},
		{
			root: trillian.SignedLogRoot{
				TimestampNanos: 2267709,
				RootHash:       []byte("Islington"),
				TreeSize:       3,
			},
		},
	} {
		hash, err := hashLogRoot(test.root)
		if err != nil {
			t.Fatalf("HashLogRoot(): %v", err)
		}

		var h [20]byte
		copy(h[:], hash)
		if _, ok := unique[h]; ok {
			t.Errorf("Found duplicate hash from input %v", test.root)
		}
		unique[h] = true
	}

}

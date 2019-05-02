// Copyright 2019 Google Inc. All Rights Reserved.
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

package licenses

import (
	"go/build"
	"testing"
)

func TestLibraryName(t *testing.T) {
	for _, test := range []struct {
		desc     string
		lib      *Library
		wantName string
	}{
		{
			desc:     "Library with no packages",
			lib:      &Library{},
			wantName: "",
		},
		{
			desc: "Library with 1 package",
			lib: &Library{
				Packages: []*build.Package{
					{ImportPath: "github.com/google/trillian/crypto"},
				},
			},
			wantName: "github.com/google/trillian/crypto",
		},
		{
			desc: "Library with 2 packages",
			lib: &Library{
				Packages: []*build.Package{
					{ImportPath: "github.com/google/trillian/crypto"},
					{ImportPath: "github.com/google/trillian/server"},
				},
			},
			wantName: "github.com/google/trillian",
		},
		{
			desc: "Vendored library",
			lib: &Library{
				Packages: []*build.Package{
					{ImportPath: "github.com/google/trillian/vendor/coreos/etcd"},
				},
			},
			wantName: "github.com/google/trillian/vendor/coreos/etcd",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			if got, want := test.lib.Name(), test.wantName; got != want {
				t.Fatalf("Name() = %q, want %q", got, want)
			}
		})
	}
}

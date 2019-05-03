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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestLibaries(t *testing.T) {
	for _, test := range []struct {
		desc       string
		importPath string
		workingDir string
		importMode build.ImportMode
		wantLibs   []string
	}{
		{
			desc:       "Detects direct dependency",
			importPath: "github.com/google/trillian/scripts/licenses/licenses/testdata/direct",
			wantLibs: []string{
				"github.com/google/trillian/scripts/licenses/licenses/testdata/indirect",
			},
		},
		{
			desc:       "Detects transitive dependency",
			importPath: "github.com/google/trillian/scripts/licenses/licenses/testdata",
			wantLibs: []string{
				"github.com/google/trillian/scripts/licenses/licenses/testdata/direct",
				"github.com/google/trillian/scripts/licenses/licenses/testdata/indirect",
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			pkg, err := build.Import(test.importPath, test.workingDir, test.importMode)
			if err != nil {
				t.Fatalf("build.Import(%q, %q, %v) = (_, %q), want (_, nil)", test.importPath, test.workingDir, test.importMode, err)
			}
			gotLibs, err := Libraries(&build.Default, pkg)
			if err != nil {
				t.Fatalf("Libraries(_, %v) = (_, %q), want (_, nil)", pkg, err)
			}
			var gotLibNames []string
			for _, lib := range gotLibs {
				gotLibNames = append(gotLibNames, lib.Name())
			}
			if diff := cmp.Diff(test.wantLibs, gotLibNames, cmpopts.SortSlices(func(x, y string) bool { return x < y })); diff != "" {
				t.Errorf("Libraries(_, %v): diff (-want +got)\n%s", pkg, diff)
			}
		})
	}
}

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

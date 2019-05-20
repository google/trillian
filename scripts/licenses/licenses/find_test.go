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
	"path/filepath"
	"testing"
)

func TestFind(t *testing.T) {
	for _, test := range []struct {
		desc            string
		importPath      string
		workingDir      string
		importMode      build.ImportMode
		wantLicensePath string
	}{
		{
			desc:            "Trillian license",
			importPath:      "github.com/google/trillian/scripts/licenses/licenses",
			wantLicensePath: filepath.Join(build.Default.GOPATH, "src/github.com/google/trillian/LICENSE"),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			pkg, err := build.Import(test.importPath, test.workingDir, test.importMode)
			if err != nil {
				t.Fatalf("build.Import(%q, %q, %v) = (_, %q), want (_, nil)", test.importPath, test.workingDir, test.importMode, err)
			}
			licensePath, err := Find(pkg)
			if err != nil || licensePath != test.wantLicensePath {
				t.Fatalf("Find(%v) = (%#v, %q), want (%q, nil)", pkg, licensePath, err, test.wantLicensePath)
			}
		})
	}
}

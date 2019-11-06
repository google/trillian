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
	classifier := classifierStub{
		licenseNames: map[string]string{
			"../../../LICENSE":         "foo",
			"testdata/licence/LICENCE": "foo",
		},
		licenseTypes: map[string]Type{
			"../../../LICENSE":         Notice,
			"testdata/licence/LICENCE": Notice,
		},
	}

	for _, test := range []struct {
		desc            string
		dir             string
		wantLicensePath string
	}{
		{
			desc:            "Trillian license",
			dir:             filepath.Join(build.Default.GOPATH, "src/github.com/google/trillian/scripts/licenses/licenses"),
			wantLicensePath: filepath.Join(build.Default.GOPATH, "src/github.com/google/trillian/LICENSE"),
		},
		{
			desc:            "Trillian licenCe",
			dir:             filepath.Join(build.Default.GOPATH, "src/github.com/google/trillian/scripts/licenses/licenses/testdata/licence"),
			wantLicensePath: filepath.Join(build.Default.GOPATH, "src/github.com/google/trillian/scripts/licenses/licenses/testdata/licence/LICENCE"),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			licensePath, err := Find(test.dir, classifier)
			if err != nil || licensePath != test.wantLicensePath {
				t.Fatalf("Find(%v) = (%#v, %q), want (%q, nil)", test.dir, licensePath, err, test.wantLicensePath)
			}
		})
	}
}

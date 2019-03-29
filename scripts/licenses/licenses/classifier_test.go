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
	"testing"
)

func TestIdentify(t *testing.T) {
	for _, test := range []struct {
		desc        string
		file        string
		confidence  float64
		wantLicense string
	}{
		{
			desc:        "Apache 2.0 license",
			file:        "testdata/apache2.txt",
			confidence:  1,
			wantLicense: "Apache-2.0",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			c, err := NewClassifier(test.confidence)
			if err != nil {
				t.Fatalf("NewClassifier(%v) = (_, %q), want (_, nil)", test.confidence, err)
			}
			license, err := c.Identify(test.file)
			if err != nil || license != test.wantLicense {
				t.Fatalf("c.Identify(%q) = (%#v, %q), want (%q, nil)", test.file, license, err, test.wantLicense)
			}
		})
	}
}

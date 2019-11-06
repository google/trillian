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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Useful in other tests in this package
type classifierStub struct {
	licenseNames map[string]string
	licenseTypes map[string]Type
	errors       map[string]error
}

func (c classifierStub) Identify(licensePath string) (string, Type, error) {
	// Convert licensePath to relative path for tests.
	wd, err := os.Getwd()
	if err != nil {
		return "", Unknown, err
	}
	relPath, err := filepath.Rel(wd, licensePath)
	if err != nil {
		return "", Unknown, err
	}
	if name, ok := c.licenseNames[relPath]; ok {
		return name, c.licenseTypes[relPath], c.errors[relPath]
	}
	if err := c.errors[relPath]; err != nil {
		return "", Unknown, c.errors[relPath]
	}
	return "", Unknown, fmt.Errorf("classifierStub has no programmed response for %q", relPath)
}

func TestIdentify(t *testing.T) {
	for _, test := range []struct {
		desc        string
		file        string
		confidence  float64
		wantLicense string
		wantType    Type
		wantErr     bool
	}{
		{
			desc:        "Apache 2.0 license",
			file:        "../../../LICENSE",
			confidence:  1,
			wantLicense: "Apache-2.0",
			wantType:    Notice,
		},
		{
			desc:       "non-existent file",
			file:       "non-existent-file",
			confidence: 1,
			wantErr:    true,
		},
		{
			desc:        "empty file path",
			file:        "",
			confidence:  1,
			wantLicense: "",
			wantType:    Unknown,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			c, err := NewClassifier(test.confidence)
			if err != nil {
				t.Fatalf("NewClassifier(%v) = (_, %q), want (_, nil)", test.confidence, err)
			}
			gotLicense, gotType, err := c.Identify(test.file)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("c.Identify(%q) = (_, _, %q), want err? %t", test.file, err, test.wantErr)
			} else if gotErr {
				return
			}
			if gotLicense != test.wantLicense || gotType != test.wantType {
				t.Fatalf("c.Identify(%q) = (%q, %q, %v), want (%q, %q, <nil>)", test.file, gotLicense, gotType, err, test.wantLicense, test.wantType)
			}
		})
	}
}

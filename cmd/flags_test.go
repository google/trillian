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

package cmd

import (
	"flag"
	"os"
	"testing"
)

func TestParseFlags(t *testing.T) {
	var a, b string
	flag.StringVar(&a, "a", "", "")
	flag.StringVar(&b, "b", "", "")

	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)

	tests := []struct {
		contents    string
		env         map[string]string
		cliArgs     []string
		expectedErr string
		expectedA   string
		expectedB   string
	}{
		{
			contents:  "-a one -b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			contents:  "-a one\n-b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			contents:  "-a one \\\n-b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			contents:  "-a one",
			cliArgs:   []string{"-b", "two"},
			expectedA: "one",
			expectedB: "two",
		},
		{
			contents:  "-a one\n-b two",
			cliArgs:   []string{"-b", "three"},
			expectedA: "one",
			expectedB: "three",
		},
		{
			contents:  "-a one\n-b $TEST_VAR",
			env:       map[string]string{"TEST_VAR": "from env"},
			expectedA: "one",
			expectedB: "from env",
		},
		{
			contents:    "-a one -b two -c three",
			expectedErr: "flag provided but not defined: -c",
		},
	}

	initalArgs := os.Args[:]
	for _, tc := range tests {
		a, b = "", ""
		os.Args = initalArgs[:]
		if len(tc.cliArgs) > 0 {
			os.Args = append(os.Args, tc.cliArgs...)
		}
		for k, v := range tc.env {
			if err := os.Setenv(k, v); err != nil {
				t.Errorf("os.SetEnv failed: %s", err)
			}
		}
		err := parseFlags(tc.contents)
		if err != nil {
			if err.Error() == tc.expectedErr {
				continue
			}
			t.Errorf("parseFlags failed: wanted: %q, got: %q", tc.expectedErr, err)
			continue
		}
		if tc.expectedA != a {
			t.Errorf("flag 'a' not properly set: wanted: %q, got %q", tc.expectedA, a)
		}
		if tc.expectedB != b {
			t.Errorf("flag 'b' not properly set: wanted: %q, got %q", tc.expectedB, b)
		}
	}
}

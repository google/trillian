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
		name        string
		contents    string
		env         map[string]string
		cliArgs     []string
		expectedErr string
		expectedA   string
		expectedB   string
	}{
		{
			name:      "two flags per line",
			contents:  "-a one -b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			name:      "one flag per line",
			contents:  "-a one\n-b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			name:      "one flag per line, with line continuation",
			contents:  "-a one \\\n-b two",
			expectedA: "one",
			expectedB: "two",
		},
		{
			name:      "one flag in file, one flag on command-line",
			contents:  "-a one",
			cliArgs:   []string{"-b", "two"},
			expectedA: "one",
			expectedB: "two",
		},
		{
			name:      "two flags, one overridden by command-line",
			contents:  "-a one\n-b two",
			cliArgs:   []string{"-b", "three"},
			expectedA: "one",
			expectedB: "three",
		},
		{
			name:      "two flags, one using an environment variable",
			contents:  "-a one\n-b $TEST_VAR",
			env:       map[string]string{"TEST_VAR": "from env"},
			expectedA: "one",
			expectedB: "from env",
		},
		{
			name:        "three flags, one undefined",
			contents:    "-a one -b two -c three",
			expectedErr: "flag provided but not defined: -c",
		},
	}

	initialArgs := os.Args[:]
	for _, tc := range tests {
		a, b = "", ""
		os.Args = append(initialArgs, tc.cliArgs...)
		for k, v := range tc.env {
			if err := os.Setenv(k, v); err != nil {
				t.Errorf("%v: os.SetEnv(%q, %q) = %q", tc.name, k, v, err)
			}
		}

		if err := parseFlags(tc.contents); err != nil {
			if err.Error() != tc.expectedErr {
				t.Errorf("%v: parseFlags() = %q, want %q", tc.name, err, tc.expectedErr)
			}
			continue
		}

		if tc.expectedA != a {
			t.Errorf("%v: flag 'a' not properly set: got %q, want %q", tc.name, a, tc.expectedA)
		}
		if tc.expectedB != b {
			t.Errorf("%v: flag 'b' not properly set: got %q, want %q", tc.name, b, tc.expectedB)
		}
	}
}

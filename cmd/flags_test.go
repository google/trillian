// Copyright 2017 Google LLC. All Rights Reserved.
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

	"github.com/google/go-cmp/cmp"
	_ "k8s.io/klog/v2"
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
			name:      "quoted flag values with spaces",
			contents:  `-a "hello world" -b 'two words'`,
			expectedA: "hello world",
			expectedB: "two words",
		},
		{
			name:        "unclosed quotation",
			contents:    `-a "unterminated`,
			expectedErr: "flag file contains unclosed quotations",
		},
		{
			name:        "trailing backslash",
			contents:    `-a one \`,
			expectedErr: "flag file contains unclosed quotations",
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

func TestSplitFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
		ok   bool
	}{
		{name: "empty", in: "", want: nil, ok: true},
		{name: "spaces only", in: "   ", want: nil, ok: true},
		{name: "tabs and newlines", in: "\t\n ", want: nil, ok: true},
		{name: "escaped space", in: `\ `, want: []string{" "}, ok: true},
		{name: "trailing escaped space", in: `a\ `, want: []string{"a "}, ok: true},
		{name: "escaped backslash", in: `\\a`, want: []string{`\a`}, ok: true},
		{name: "escaped double quote in double quotes", in: `"a\"b"`, want: []string{`a"b`}, ok: true},
		{name: "escaped backslash in double quotes", in: `"a\\b"`, want: []string{`a\b`}, ok: true},
		{name: "non-special backslash in double quotes preserved", in: `"a\nb\ c\'d"`, want: []string{`a\nb\ c\'d`}, ok: true},
		{name: "backslash literal in single quotes", in: `'\'`, want: []string{`\`}, ok: true},
		{name: "leading and trailing whitespace", in: " \ta\n ", want: []string{"a"}, ok: true},
		{name: "line continuation inside word", in: "a\\\nb", want: []string{"ab"}, ok: true},
		{name: "line continuation between words", in: "a \\\n  b\tc", want: []string{"a", "b", "c"}, ok: true},
		{name: "line continuation inside double quotes", in: "\"a\\\nb\"", want: []string{"ab"}, ok: true},
		{name: "line continuation only", in: "\\\n", want: nil, ok: true},
		{name: "line continuation after empty quotes", in: "''\\\n b", want: []string{"", "b"}, ok: true},
		{name: "single quoted whitespace", in: `a 'b c'`, want: []string{"a", "b c"}, ok: true},
		{name: "adjacent quoted and unquoted segments", in: "\"a\nb\"cd e'f'", want: []string{"a\nbcd", "ef"}, ok: true},
		{name: "single quoted multiline whitespace", in: "'\n \t '", want: []string{"\n \t "}, ok: true},
		{name: "empty single quotes", in: "''", want: []string{""}, ok: true},
		{name: "trailing empty single quotes", in: "a ''", want: []string{"a", ""}, ok: true},
		{name: "middle empty double quotes", in: ` a "" b `, want: []string{"a", "", "b"}, ok: true},
		{name: "leading empty single quotes", in: "'' a", want: []string{"", "a"}, ok: true},
		{name: "hash not treated as comment", in: "--uri=mysql://u:p#1@h/db #tag", want: []string{"--uri=mysql://u:p#1@h/db", "#tag"}, ok: true},
		{name: "unterminated escape", in: `\`, want: nil, ok: false},
		{name: "unterminated single quote", in: `'`, want: nil, ok: false},
		{name: "unterminated double quote", in: `"`, want: nil, ok: false},
		{name: "single quote then unterminated escape", in: `'\''`, want: nil, ok: false},
		{name: "unterminated trailing single quote", in: `"\\" '`, want: nil, ok: false},
		{name: "unterminated single quoted arg", in: "a 'b c", want: nil, ok: false},
		{name: "unterminated double quoted arg", in: `a "b c`, want: nil, ok: false},
		{name: "unterminated escaped quote in double quotes", in: `a "b \"`, want: nil, ok: false},
		{name: "unterminated backslash in double quotes", in: `"a\`, want: nil, ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := splitFlags(tc.in)
			if ok != tc.ok {
				t.Fatalf("splitFlags(%q) ok = %v, want %v (got %q)", tc.in, ok, tc.ok, got)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("splitFlags(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

func FuzzSplitFlags(f *testing.F) {
	seeds := []string{
		"",
		"-a one -b two",
		"-a \"hello world\" \\\n-b 'single\\quoted'",
		`"" '' "a\"b\\c\d"`,
		`\`,
		`"unclosed`,
		`'unclosed`,
		`"trailing\`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, ok := splitFlags(in)
		if !ok && got != nil {
			t.Fatalf("splitFlags(%q) returned non-nil tokens %q when ok=false", in, got)
		}
	})
}

func BenchmarkSplitFlags(b *testing.B) {
	const sample = "--storage_system=mysql \\\n--mysql_uri=\"trillian:secret@tcp(127.0.0.1:3306)/trillian\" \\\n--rpc_endpoint='localhost:8090' --http_endpoint=localhost:8091 --alsologtostderr\n"
	for b.Loop() {
		_, _ = splitFlags(sample)
	}
}

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

// Package cmd contains common code for the various binaries in this repository.
package cmd

import (
	"errors"
	"flag"
	"os"
	"strings"
)

func parseFlags(file string) error {
	args, valid := splitFlags(file)
	if !valid {
		return errors.New("flag file contains unclosed quotations")
	}
	// Expand any environment variables in the args
	for i := range args {
		args[i] = os.ExpandEnv(args[i])
	}

	if err := flag.CommandLine.Parse(args); err != nil {
		return err
	}

	// Call flag.Parse() again so that command line flags
	// can override flags provided in the provided flag file.
	flag.Parse()
	return nil
}

// ParseFlagFile parses a set of flags from a file at the provided
// path. Re-calls flag.Parse() after parsing the flags in the file
// so that flags provided on the command line take precedence over
// flags provided in the file.
func ParseFlagFile(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return parseFlags(string(file))
}

// splitFlags splits s into whitespace-delimited tokens, honoring single quotes,
// double quotes, and backslash escapes (including \<newline> line continuations).
// It returns false if s ends with an unclosed quote or trailing backslash.
func splitFlags(s string) ([]string, bool) {
	var (
		tokens []string
		cur    strings.Builder
		inWord bool
		quote  byte // 0, '\'', or '"'
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		if quote == '"' {
			if c == '"' {
				quote = 0
				continue
			}
			if c == '\\' {
				i++
				if i >= len(s) {
					return nil, false
				}
				writeDoubleQuoteEscape(&cur, s[i])
				continue
			}
			cur.WriteByte(c)
			continue
		}

		switch c {
		case ' ', '\t', '\n':
			if inWord {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inWord = false
			}
		case '\'', '"':
			quote = c
			inWord = true
		case '\\':
			i++
			if i >= len(s) {
				return nil, false
			}
			if s[i] != '\n' {
				cur.WriteByte(s[i])
				inWord = true
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inWord {
		tokens = append(tokens, cur.String())
	}
	return tokens, true
}

func writeDoubleQuoteEscape(cur *strings.Builder, c byte) {
	switch c {
	case '\n':
		// Line continuation inside double quotes.
	case '\\', '"':
		cur.WriteByte(c)
	default:
		cur.WriteByte('\\')
		cur.WriteByte(c)
	}
}

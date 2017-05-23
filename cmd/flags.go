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
	"io/ioutil"

	"github.com/mattn/go-shellwords"
)

// ParseFlagFile parses a set of flags from a file at the provided
// path. Re-calls flag.Parse() after parsing the flags in the file
// so that flags provided on the command line take precedence over
// flags provided in the file.
func ParseFlagFile(path string) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	p := shellwords.NewParser()
	p.ParseEnv = true
	args, err := p.Parse(string(file))
	if err != nil {
		return err
	}

	err = flag.CommandLine.Parse(args)
	if err != nil {
		return err
	}

	// Call flag.Parse() again so that command line flags
	// can override flags provided in the provided flag file.
	flag.Parse()
	return nil
}

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

package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"strings"

	"github.com/google/trillian/scripts/licenses/licenses"

	"bitbucket.org/creachadair/shell"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	confidenceThreshold float64
	buildTags           string

	rootCmd = &cobra.Command{
		Use: "licenses",
	}
)

func init() {
	rootCmd.PersistentFlags().Float64Var(&confidenceThreshold, "confidence_threshold", 0.9, "Minimum confidence required in order to positively identify a license.")
	// Go build flags, which should match flags offered by `go build`
	rootCmd.PersistentFlags().StringVar(&buildTags, "tags", "", "A space-separated list of build tags to consider satisfied.")
}

func main() {
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	if err := parseGoBuildFlags(rootCmd.PersistentFlags()); err != nil {
		glog.Error(err)
	}

	if err := rootCmd.Execute(); err != nil {
		glog.Exit(err)
	}
}

// Libraries returns the libraries used by the package identified by importPath.
// The import path is assumed to be in the context of the current working
// directory, so vendoring and relative import paths will work.
func libraries(importPath string) ([]*licenses.Library, error) {
	// Import the main package and find all of the libraries that it uses.
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	pkg, err := build.Import(importPath, wd, build.ImportMode(0))
	if err != nil {
		return nil, err
	}
	buildCtx := build.Default
	buildCtx.BuildTags = append(buildCtx.BuildTags, strings.Split(buildTags, " ")...)
	return licenses.Libraries(&buildCtx, pkg)
}

// ParseGoBuildFlags will parse the $GOFLAGS environment variable for recognised
// flags and adopt their values.
func parseGoBuildFlags(flagset *pflag.FlagSet) error {
	// Temporarily ensure that unknown flags are not treated as an error, because
	// this binary doesn't support most `go build` flags.
	defer func(oldValue bool) {
		flagset.ParseErrorsWhitelist.UnknownFlags = oldValue
	}(flagset.ParseErrorsWhitelist.UnknownFlags)
	rootCmd.PersistentFlags().ParseErrorsWhitelist.UnknownFlags = true

	goFlags, ok := shell.Split(os.Getenv("GOFLAGS"))
	if !ok {
		return fmt.Errorf("$GOFLAGS is invalid: unclosed quotation")
	}
	return flagset.Parse(goFlags)
}

// Unvendor removes the "*/vendor/" prefix from the given import path, if present.
func unvendor(importPath string) string {
	if vendorerAndVendoree := strings.SplitN(importPath, "/vendor/", 2); len(vendorerAndVendoree) == 2 {
		return vendorerAndVendoree[1]
	}
	return importPath
}

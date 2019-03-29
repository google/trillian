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
	"os"
	"strings"

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
		fmt.Println(err)
		os.Exit(1)
	}
}

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

func unvendor(importPath string) string {
	if vendorerAndVendoree := strings.SplitN(importPath, "/vendor/", 2); len(vendorerAndVendoree) == 2 {
		return vendorerAndVendoree[1]
	}
	return importPath
}

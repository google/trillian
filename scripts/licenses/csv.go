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
	"encoding/csv"
	"go/build"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian/scripts/licenses/licenses"
	"github.com/spf13/cobra"
)

var (
	gitRemotes []string

	csvCmd = &cobra.Command{
		Use:   "csv <package>",
		Short: "Prints all licenses that apply to a Go package and its dependencies",
		Args:  cobra.ExactArgs(1),
		RunE:  csvMain,
	}
)

func init() {
	csvCmd.Flags().StringArrayVar(&gitRemotes, "git_remote", []string{"origin", "upstream"}, "Remote Git repositories to try")

	rootCmd.AddCommand(csvCmd)
}

func csvMain(cmd *cobra.Command, args []string) error {
	writer := csv.NewWriter(os.Stdout)
	// Import the main package and find all of the libraries that it uses.
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	pkg, err := build.Import(args[0], wd, 0)
	if err != nil {
		return err
	}
	classifier, err := licenses.NewClassifier(confidenceThreshold)
	if err != nil {
		return err
	}
	buildCtx := build.Default
	buildCtx.BuildTags = append(buildCtx.BuildTags, strings.Split(buildTags, " ")...)
	libs, err := licenses.Libraries(&buildCtx, pkg)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		licenseURL := "Unknown"
		licenseName := "Unknown"
		if lib.LicensePath != "" {
			// Find a URL for the license file, based on the URL of a remote for the Git repository.
			var errs []error
			for _, remote := range gitRemotes {
				url, err := licenses.GitFileURL(lib.LicensePath, remote)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				licenseURL = url.String()
				break
			}
			if licenseURL == "Unknown" {
				glog.Errorf("Error discovering URL for %q: %v", lib.LicensePath, errs)
			}
			licenseName, _, err = classifier.Identify(lib.LicensePath)
			if err != nil {
				return err
			}
		}
		// Remove the "*/vendor/" prefix from the library name for conciseness.
		if err := writer.Write([]string{unvendor(lib.Name()), licenseURL, licenseName}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

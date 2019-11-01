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
	"context"
	"fmt"
	"os"

	"github.com/google/trillian/scripts/licenses/licenses"
	"github.com/spf13/cobra"
)

var (
	checkCmd = &cobra.Command{
		Use:   "check <package>",
		Short: "Checks whether licenses for a package are not Forbidden.",
		Args:  cobra.ExactArgs(1),
		RunE:  checkMain,
	}
)

func init() {
	rootCmd.AddCommand(checkCmd)
}

func checkMain(_ *cobra.Command, args []string) error {
	classifier, err := licenses.NewClassifier(confidenceThreshold)
	if err != nil {
		return err
	}

	importPath := args[0]
	libs, err := licenses.Libraries(context.Background(), importPath)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		licenseName, licenseType, err := classifier.Identify(lib.LicensePath)
		if err != nil {
			return err
		}
		if licenseType == licenses.Forbidden {
			fmt.Fprintf(os.Stderr, "Forbidden license type %s for library %v", licenseName, lib)
			os.Exit(1)
		}
	}
	return nil
}

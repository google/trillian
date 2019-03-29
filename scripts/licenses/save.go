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
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian/scripts/licenses/licenses"
	"github.com/otiai10/copy"
	"github.com/spf13/cobra"
)

var (
	noticeRegexp = regexp.MustCompile(`^NOTICE(\.(txt|md))?$`)

	savePath string

	saveCmd = &cobra.Command{
		Use:   "save <package>",
		Short: "Prints all licenses that apply to a Go package and its dependencies",
		Args:  cobra.ExactArgs(1),
		RunE:  saveMain,
	}
)

func init() {
	saveCmd.Flags().StringVar(&savePath, "save_path", "", "Directory into which files should be saved that are required by license terms")
	if err := saveCmd.MarkFlagRequired("save_path"); err != nil {
		glog.Fatal(err)
	}
	if err := saveCmd.MarkFlagFilename("save_path"); err != nil {
		glog.Fatal(err)
	}

	rootCmd.AddCommand(saveCmd)
}

func saveMain(cmd *cobra.Command, args []string) error {
	if d, err := os.Open(savePath); err == nil {
		d.Close()
		return fmt.Errorf("%s already exists", savePath)
	} else if !os.IsNotExist(err) {
		return err
	}
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
	var unlicensedPkgs []*build.Package
	buildCtx := build.Default
	buildCtx.BuildTags = append(buildCtx.BuildTags, strings.Split(buildTags, " ")...)
	libs, err := licenses.Libraries(&buildCtx, pkg)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		if lib.LicensePath == "" {
			unlicensedPkgs = append(unlicensedPkgs, lib.Packages...)
			continue
		}
		libDir := filepath.Dir(lib.LicensePath)
		libSaveDir := filepath.Join(savePath, unvendor(lib.Name()))
		// Detect what type of license this library has and fulfill its requirements, e.g. copy license, copyright notice, source code, etc.
		licenseName, licenseType, err := lib.ClassifyLicense(classifier)
		if err != nil {
			return err
		}
		copySrc := false
		switch licenseType {
		case licenses.Restricted, licenses.Reciprocal:
			copySrc = true
		case licenses.Notice, licenses.Permissive, licenses.Unencumbered:
			copySrc = false
		case licenses.Forbidden:
			return fmt.Errorf("forbidden license %q used by this library: %s", licenseName, lib)
		default:
			return fmt.Errorf("%q license is of an unknown type", licenseName)
		}
		if copySrc {
			if err := copy.Copy(libDir, libSaveDir); err != nil {
				return err
			}
			// Delete the .git directory from the saved copy, if it exists, since we don't want to save the user's
			// local Git config along with the source code.
			if err := os.RemoveAll(filepath.Join(libSaveDir, ".git")); err != nil {
				return err
			}
		} else {
			// Just copy the license and copyright notice.
			if err := copy.Copy(lib.LicensePath, filepath.Join(libSaveDir, filepath.Base(lib.LicensePath))); err != nil {
				return err
			}

			files, err := ioutil.ReadDir(libDir)
			if err != nil {
				return err
			}
			for _, f := range files {
				if fName := f.Name(); !f.IsDir() && noticeRegexp.MatchString(fName) {
					if err := copy.Copy(filepath.Join(libDir, fName), filepath.Join(libSaveDir, fName)); err != nil {
						return err
					}
				}
			}
		}
	}
	if len(unlicensedPkgs) > 0 {
		return fmt.Errorf("Unlicensed packages: %v", unlicensedPkgs)
	}
	return nil
}

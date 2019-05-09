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

package licenses

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"path/filepath"
	"regexp"
)

// Find returns the file path of the license for this package.
func Find(pkg *build.Package) (string, error) {
	var stopAt []*regexp.Regexp
	stopAt = append(stopAt, srcDirRegexps...)
	stopAt = append(stopAt, vendorRegexp)
	return findUpwards(pkg.Dir, licenseRegexp, stopAt)
}

func findUpwards(dir string, r *regexp.Regexp, stopAt []*regexp.Regexp) (string, error) {
	start := dir
	for !matchAny(stopAt, dir) {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, f := range files {
			if r.MatchString(f.Name()) {
				return filepath.Join(dir, f.Name()), nil
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("no file matching %q found for %s", r, start)
}

func matchAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

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

var (
	licenseRegexp = regexp.MustCompile(`^LICEN(S|C)E(\.(txt|md))?$`)
	srcDirRegexps = func() []*regexp.Regexp {
		var rs []*regexp.Regexp
		for _, s := range build.Default.SrcDirs() {
			rs = append(rs, regexp.MustCompile("^"+regexp.QuoteMeta(s)+"$"))
		}
		return rs
	}()
	vendorRegexp = regexp.MustCompile(`.+/vendor(/)?$`)
)

// Find returns the file path of the license for this package.
func Find(dir string, classifier Classifier) (string, error) {
	var stopAt []*regexp.Regexp
	stopAt = append(stopAt, srcDirRegexps...)
	stopAt = append(stopAt, vendorRegexp)
	return findUpwards(dir, licenseRegexp, stopAt, func(path string) bool {
		// TODO(RJPercival): Return license details
		if _, _, err := classifier.Identify(path); err != nil {
			return false
		}
		return true
	})
}

func findUpwards(dir string, r *regexp.Regexp, stopAt []*regexp.Regexp, predicate func(path string) bool) (string, error) {
	start := dir
	// Stop once dir matches a stopAt regexp or dir is the filesystem root
	for !matchAny(stopAt, dir) {
		dirContents, err := ioutil.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, f := range dirContents {
			if r.MatchString(f.Name()) {
				path := filepath.Join(dir, f.Name())
				if predicate != nil && !predicate(path) {
					continue
				}
				return path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Can't go any higher up the directory tree.
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no file/directory matching regexp %q found for %s", r, start)
}

func matchAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

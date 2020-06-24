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

package testonly

import (
	"path"
	"path/filepath"
	"runtime"
)

// RelativeToPackage returns the input path p as an absolute path, resolved relative to the caller's package.
// The working directory for Go tests is the dir of the test file. Using "plain" relative paths in test
// utilities is, therefore, brittle, as the directory structure may change depending on where the tests are placed.
func RelativeToPackage(p string) string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("cannot get caller information")
	}

	absPath, err := filepath.Abs(filepath.Join(path.Dir(file), p))
	if err != nil {
		panic(err)
	}
	return absPath
}

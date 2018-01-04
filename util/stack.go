// Copyright 2018 Google Inc. All Rights Reserved.
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

package util

import (
	"runtime"

	"github.com/golang/glog"
)

// LogStackTraces logs the stack traces of all executing goroutines.
func LogStackTraces(prefix string) {
	b := make([]byte, 64<<10)
	s := runtime.Stack(b, true)
	glog.Infof("%s: %s", prefix, string(b[:s]))
}

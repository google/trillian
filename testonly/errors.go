// Copyright 2016 Google LLC. All Rights Reserved.
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
	"strings"
	"testing"
)

// EnsureErrorContains checks that an error contains a specific substring and fails a
// test with a fatal if it does not or the error was nil.
func EnsureErrorContains(t *testing.T, err error, s string) {
	if err == nil {
		t.Fatalf("%s operation unexpectedly succeeded", s)
	}

	if !strings.Contains(err.Error(), s) {
		t.Fatalf("Got the wrong type of error: %v", err)
	}
}

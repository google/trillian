// Copyright 2018 Google LLC. All Rights Reserved.
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

package setup

import (
	"os"
	"testing"
)

// TempFile creates a temporary file to be used in a test, and returns its
// *os.File handler, as well as a cleanup function.
func TempFile(t *testing.T, prefix string) (*os.File, func()) {
	t.Helper()

	file, err := os.CreateTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to generate a temporary file: %v", err)
	}

	cleanup := func() {
		if err := os.Remove(file.Name()); err != nil {
			t.Fatalf("Failed to remove temporary file '%s': %v", file.Name(), err)
		}

		if err := file.Close(); err != nil {
			t.Fatalf("Failed to close temporary file handler '%v': %v", file, err)
		}
	}

	return file, cleanup
}

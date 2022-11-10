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

package crdb

import (
	"flag"
	"testing"

	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly/flagsaver"
)

func TestCockroachDBStorageProviderErrorPersistence(t *testing.T) {
	t.Parallel()

	defer flagsaver.Save().MustRestore()
	if err := flag.Set("crdb_uri", "&bogus*:::?"); err != nil {
		t.Errorf("Failed to set flag: %v", err)
	}

	// First call: This should fail due to the Database URL being garbage.
	_, err1 := storage.NewProvider(StorageProviderName, nil)
	if err1 == nil {
		t.Fatalf("Expected 'storage.NewProvider' to fail")
	}

	// Second call: This should fail with the same error.
	_, err2 := storage.NewProvider(StorageProviderName, nil)
	if err2 == nil {
		t.Fatalf("Expected second call to 'storage.NewProvider' to fail")
	}

	if err2 != err1 {
		t.Fatalf("Expected second call to 'storage.NewProvider' to fail with %q, instead got: %q", err1, err2)
	}
}

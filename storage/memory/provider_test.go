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

package memory

import (
	"testing"

	"github.com/google/trillian/storage"
)

func TestMemoryStorageProvider(t *testing.T) {
	sp, err := storage.NewProvider("memory", nil)
	if err != nil {
		t.Fatalf("Got an unexpected error: %v", err)
	}
	if sp == nil {
		t.Fatal("Got a nil storage provider.")
	}
}

func TestMemoryStorageProviderLogStorage(t *testing.T) {
	sp, err := storage.NewProvider("memory", nil)
	if err != nil {
		t.Fatalf("Got an unexpected error: %v", err)
	}

	ls := sp.LogStorage()
	if ls == nil {
		t.Fatal("Got a nil log storage interface.")
	}
}

func TestMemoryStorageProviderAdminStorage(t *testing.T) {
	sp, err := storage.NewProvider("memory", nil)
	if err != nil {
		t.Fatalf("Got an unexpected error: %v", err)
	}

	as := sp.AdminStorage()
	if as == nil {
		t.Fatal("Got a nil admin storage interface.")
	}
}

func TestMemoryStorageProviderClose(t *testing.T) {
	sp, err := storage.NewProvider("memory", nil)
	if err != nil {
		t.Fatalf("Got an unexpected error: %v", err)
	}

	err = sp.Close()
	if err != nil {
		t.Fatalf("Failed to close the memory storage provider: %v", err)
	}
}

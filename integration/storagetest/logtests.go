// Copyright 2020 Google Inc. All Rights Reserved.
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

package storagetest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/storage"
)

// LogStorageFactory creates LogStorage and AdminStorage for a test to use.
type LogStorageFactory = func(ctx context.Context, t *testing.T) (storage.LogStorage, storage.AdminStorage)

// LogStorageTest executes a test using the given storage implementations.
type LogStorageTest = func(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage)

// RunLogStorageTests runs all the log storage tests against the provided log storage implementation.
func RunLogStorageTests(t *testing.T, storageFactory LogStorageFactory) {
	ctx := context.Background()
	for name, f := range logTestFunctions(t, &LogTests{}) {
		s, as := storageFactory(ctx, t)
		t.Run(name, func(t *testing.T) { f(ctx, t, s, as) })
	}
}

func logTestFunctions(t *testing.T, x interface{}) map[string]LogStorageTest {
	tests := make(map[string]LogStorageTest)
	xv := reflect.ValueOf(x)
	for _, name := range testFunctions(x) {
		m := xv.MethodByName(name)
		if !m.IsValid() {
			t.Fatalf("storagetest: function %v is not valid", name)
		}
		i := m.Interface()
		f, ok := i.(LogStorageTest)
		if !ok {
			// Method exists but has the wrong type signature.
			t.Fatalf("storagetest: function %v has unexpected signature %T, %v", name, m.Interface(), m)
		}
		nickname := strings.TrimPrefix(name, "Test")
		tests[nickname] = f
	}
	return tests
}

// LogTests is a suite of tests to run against the storage.LogTest interface.
type LogTests struct{}

func (*LogTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

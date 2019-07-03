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

package storagetest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/storage"
)

type MapStorageFactory func(ctx context.Context, t *testing.T) (storage.MapStorage, storage.AdminStorage)
type MapStorageTest func(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage)

// RunMapStorageTests runs all the map storage tests against the provided map storage implementation.
func RunMapStorageTests(t *testing.T, storageFactory MapStorageFactory) {
	ctx := context.Background()
	for name, f := range mapTestFunctions(t, &MapTests{}) {
		ms, as := storageFactory(ctx, t)
		t.Run(name, func(t *testing.T) { f(ctx, t, ms, as) })
	}
}

func mapTestFunctions(t *testing.T, x interface{}) map[string]MapStorageTest {
	prefix := "Test"
	xt := reflect.TypeOf(x)
	xv := reflect.ValueOf(x)

	tests := make(map[string]MapStorageTest)
	for i := 0; i < xt.NumMethod(); i++ {
		methodName := xt.Method(i).Name
		if !strings.HasPrefix(methodName, prefix) {
			continue
		}
		name := strings.TrimPrefix(methodName, "Test")
		tests[name] = getTestFunc(t, xv, methodName)
	}
	return tests
}

func getMapTestFunc(t *testing.T, xv reflect.Value, name string) MapStorageTest {
	m := xv.MethodByName(name)
	if !m.IsValid() {
		t.Fatalf("storagetest: function %v is not valid", name)
	}
	f, ok := m.Interface().(func(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage))
	if !ok {
		// Method exists but has the wrong type signature.
		t.Fatalf("storagetest: function %v has unexpected signature (%T), want %v", name, m.Interface(), m)
	}
	return f
}

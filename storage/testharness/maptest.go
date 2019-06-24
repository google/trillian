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

// Package testharness verifies that storage interfaces behave correctly
package testharness

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/google/trillian/storage"
)

func TestMapStorage(t *testing.T, s storage.MapStorage) {
	ctx := context.Background()
	for _, f := range []func(context.Context, *testing.T, storage.MapStorage){
		CheckDatabaseAccessible,
	} {
		t.Run(functionName(f), func(t *testing.T) { f(ctx, t, s) })
	}
}

func functionName(i interface{}) string {
	pc := reflect.ValueOf(i).Pointer()
	nameFull := runtime.FuncForPC(pc).Name() // main.foo
	nameEnd := filepath.Ext(nameFull)        // .foo
	name := strings.TrimPrefix(nameEnd, ".") // foo
	return name
}

func CheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.MapStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

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

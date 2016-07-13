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
		t.Errorf("Got the wrong type of error: %v", err)
	}
}


package util

import (
	"testing"

	"golang.org/x/net/context"
)

var tests = []struct {
	id   int64 // -1 for no NewLogContext
	want string
}{
	{-1, "{unknown}"},
	{3, "{3}"},
	{5, "{5}"},
}

func TestLogContext(t *testing.T) {
	// ctx is deliberately re-used between test cases.
	ctx := context.Background()
	for _, test := range tests {
		if test.id != -1 {
			ctx = NewLogContext(ctx, test.id)
		}
		got := LogIDPrefix(ctx)
		if got != test.want {
			t.Errorf("LogID(ctx)=%q; want %q", got, test.want)
		}
	}
}
func TestMapContext(t *testing.T) {
	// ctx is deliberately re-used between test cases.
	ctx := context.Background()
	for _, test := range tests {
		if test.id != -1 {
			ctx = NewMapContext(ctx, test.id)
		}
		got := MapIDPrefix(ctx)
		if got != test.want {
			t.Errorf("MapID(ctx)=%q; want %q", got, test.want)
		}
	}
}

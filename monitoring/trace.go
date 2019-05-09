package monitoring

import (
	"context"
	"sync"
)

// StartSpan is the global entry point for Trillian code to create new tracing spans.
var (
	once      sync.Once
	startSpan StartSpanFunc = func(ctx context.Context, _ string) (context.Context, func()) { return ctx, func() {} }
)

// SetStartSpanFunc allows the tracing span implementation to be set.
// This function will set the global tracing function to the one supplied by
// the first caller, future calls to this function will have no effect.
func SetStartSpanFunc(s StartSpanFunc) {
	once.Do(func() {
		startSpan = s
	})
}

// StartSpan starts a new tracing span.
func StartSpan(ctx context.Context, name string) (context.Context, func()) {
	return startSpan(ctx, name)
}

// StartSpanFunc is the signature of a function which starts new tracing spans.
type StartSpanFunc func(ctx context.Context, name string) (context.Context, func())

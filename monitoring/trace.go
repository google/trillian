// Copyright 2019 Google LLC. All Rights Reserved.
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

package monitoring

import "context"

// startSpanFunc is the signature of a function which can start tracing spans.
type startSpanFunc func(context.Context, string) (context.Context, func())

var startSpan startSpanFunc = noopStartSpan

// noopStartSpan is a span starting function which does nothing, and is used as
// the default implementation.
func noopStartSpan(ctx context.Context, _ string) (context.Context, func()) {
	return ctx, func() {}
}

// StartSpan starts a new tracing span using the given message.
// The returned context should be used for all child calls within the span, and
// the returned func should be called to close the span.
//
// The default implementation of this method is a no-op; insert a real tracing span
// implementation by setting this global variable to the relevant function at start of day.
func StartSpan(ctx context.Context, name string) (context.Context, func()) {
	return startSpan(ctx, name)
}

// SetStartSpan sets the function used to start tracing spans.
// This may be used to add runtime support for different tracing implementation.
func SetStartSpan(f startSpanFunc) {
	startSpan = f
}

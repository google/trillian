// Copyright 2017 Google Inc. All Rights Reserved.
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

package util

import (
	"fmt"

	"golang.org/x/net/context"
)

type contextKey int

const (
	// logIDKey is the key used when storing a LogID in a context.Context.
	logIDKey contextKey = iota

	// mapIDKey is the key used when storing a MapID in a context.Context.
	mapIDKey contextKey = iota
)

// NewLogContext returns a new context instance that is scoped to a particular Log.
func NewLogContext(ctx context.Context, logID int64) context.Context {
	return context.WithValue(ctx, logIDKey, logID)
}

// NewMapContext returns a new context instance that is scoped to a particular Map.
func NewMapContext(ctx context.Context, mapID int64) context.Context {
	return context.WithValue(ctx, mapIDKey, mapID)
}

// LogIDPrefix returns an identifier for the log associated with ctx in a form
// suitable for use as a diagnostic prefix.
func LogIDPrefix(ctx context.Context) string {
	v, ok := ctx.Value(logIDKey).(int64)
	if !ok {
		return "{unknown}"
	}
	return fmt.Sprintf("{%d}", v)
}

// MapIDPrefix returns an identifier for the log associated with ctx in a form
// suitable for use as a diagnostic prefix.
func MapIDPrefix(ctx context.Context) string {
	v, ok := ctx.Value(mapIDKey).(int64)
	if !ok {
		return "{unknown}"
	}
	return fmt.Sprintf("{%d}", v)
}

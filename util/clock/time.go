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

package clock

import (
	"context"
	"time"
)

// SleepContext sleeps for at least the specified duration. Returns ctx.Err()
// iff the context is done before the deadline.
func SleepContext(ctx context.Context, d time.Duration) error {
	if !sleepImpl(ctx.Done(), d, System) {
		return ctx.Err()
	}
	return nil
}

// SleepSource sleeps for at least the specified duration, as measured by the
// TimeSource. Returns ctx.Err() iff the context is done before the deadline.
func SleepSource(ctx context.Context, d time.Duration, s TimeSource) error {
	if !sleepImpl(ctx.Done(), d, s) {
		return ctx.Err()
	}
	return nil
}

// sleepImpl sleeps for at least the specified duration, as measured by the
// TimeSource. Returns false iff done is closed before the deadline.
func sleepImpl(done <-chan struct{}, d time.Duration, s TimeSource) bool {
	timer := s.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.Chan():
		return true
	case <-done:
		return false
	}
}

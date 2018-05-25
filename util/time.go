// Copyright 2018 Google Inc. All Rights Reserved.
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
	"context"
	"time"
)

// Sleep sleeps for at least the specified duration. Returns an error iff done
// is closed before the timer fires.
func Sleep(done <-chan struct{}, dur time.Duration) error {
	timer := time.NewTimer(dur)
	select {
	case <-timer.C:
	case <-done:
		timer.Stop()
		return context.Canceled
	}
	return nil
}

// SleepContext sleeps for at least the specified duration. Returns ctx.Err()
// iff the context is canceled before the timer fires.
func SleepContext(ctx context.Context, dur time.Duration) error {
	if err := Sleep(ctx.Done(), dur); err != nil {
		return ctx.Err()
	}
	return nil
}

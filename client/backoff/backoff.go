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

// Package backoff allows retrying an operation with backoff.
package backoff

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// Backoff specifies the parameters of the backoff algorithm.
type Backoff struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter bool
	x      int
}

// Duration returns the time to wait on duration x.
// Every time Duration is called, the returned value will exponentially increase by Factor
// until Backoff.Max. If Jitter is enabled, will wait an additional random value between
// 0 and factor^x * min, capped by Backoff.Max.
func (b *Backoff) Duration() time.Duration {
	// min( min * factor ^ x , max)
	minNanos := float64(b.Min.Nanoseconds())
	maxNanos := float64(b.Max.Nanoseconds())
	nanos := minNanos * math.Pow(b.Factor, float64(b.x))
	nanos = math.Min(nanos, maxNanos)
	if b.Jitter {
		// Generate a number in the range [0, factor^x * b.Min)
		r := float64(rand.Int63n(int64(nanos)))
		nanos += r
	}
	b.x++
	return time.Duration(nanos) * time.Nanosecond
}

// Reset sets the internal iteration count back to 0.
func (b *Backoff) Reset() {
	b.x = 0
}

// Retry calls a function until it succeeds or the context is done.
// It will backoff if the function returns an error.
// Once the context is done, retries will end and the most recent error will be returned.
// Backoff is not reset by this function.
func (b *Backoff) Retry(ctx context.Context, f func() error) error {
	// If the context is already done, don't make any attempts to call f.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Try calling f until it doesn't return an error or ctx is done.
	for {
		err := f()
		if err != nil {
			select {
			case <-time.After(b.Duration()):
				continue
			case <-ctx.Done():
				return err
			}
		}

		return nil
	}
}

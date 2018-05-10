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

// Backoff specifies the parameters of the backoff algorithm. Works correctly
// if 0 < Min <= Max <= 2^62 (nanosec), and Factor >= 1.
type Backoff struct {
	Min    time.Duration // Duration of the first pause.
	Max    time.Duration // Max duration of a pause.
	Factor float64       // The factor of duration increase between iterations.
	Jitter bool          // Add random noise to pauses.

	delta time.Duration // Current pause duration relative to Min, no jitter.
}

// Duration returns the time to wait on current retry iteration.
// Every time Duration is called, the returned value will exponentially
// increase by Factor until Backoff.Max. If Jitter is enabled, will wait an
// additional random value between 0 and Factor^x * Min, capped by Backoff.Max.
func (b *Backoff) Duration() time.Duration {
	pause := b.Min + b.delta

	newPause := time.Duration(float64(pause) * b.Factor)
	if newPause > b.Max || newPause < b.Min { // Multiplication could overflow.
		newPause = b.Max
	}
	b.delta = newPause - b.Min

	if b.Jitter {
		// Add a number in the range [0, pause).
		pause += time.Duration(rand.Int63n(int64(pause)))
	}
	return pause
}

// Reset sets the internal state back to first iteration.
func (b *Backoff) Reset() {
	b.delta = 0
}

// Recover reduces retry pause duration by the given power of Backoff.Factor.
// This might be useful when calling Retry multiple times, to gradually adapt
// to the rate of errors: in some use-cases a successfully returned Retry could
// indicate that a bigger flow of requests can now be handled.
func (b *Backoff) Recover(factors int) {
	pause := float64(b.Min + b.delta)
	newPause := pause / math.Pow(b.Factor, float64(factors))
	minNanos := float64(b.Min)
	b.delta = time.Duration(math.Max(minNanos, newPause) - minNanos)
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
		if err := f(); err != nil {
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

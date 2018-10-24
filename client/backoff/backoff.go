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
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// Duration returns the time to wait on current retry iteration. Every time
// Duration is called, the returned value will exponentially increase by Factor
// until Backoff.Max. If Jitter is enabled, will add an additional random value
// between 0 and the duration, so the result can at most double.
func (b *Backoff) Duration() time.Duration {
	base := b.Min + b.delta
	pause := base
	if b.Jitter { // Add a number in the range [0, pause).
		pause += time.Duration(rand.Int63n(int64(pause)))
	}

	nextPause := time.Duration(float64(base) * b.Factor)
	if nextPause > b.Max || nextPause < b.Min { // Multiplication could overflow.
		nextPause = b.Max
	}
	b.delta = nextPause - b.Min

	return pause
}

// Reset sets the internal state back to first retry iteration.
func (b *Backoff) Reset() {
	b.delta = 0
}

// Retry calls a function until it succeeds or the context is done.
// It will backoff if the function returns a retryable error.
// Once the context is done, retries will end and the most recent error will be returned.
// Backoff is not reset by this function.
func (b *Backoff) Retry(ctx context.Context, f func() error) error {
	// If the context is already done, don't make any attempts to call f.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Try calling f while the error is retryable and ctx is not done.
	for {
		if err := f(); IsRetryable(err) {
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

// IsRetryable returns true unless the error is explicitly not retriable per
// https://godoc.org/google.golang.org/grpc/codes.
func IsRetryable(err error) bool {
	switch code := status.Code(err); code {

	// Errors that should not be retried:
	case codes.OK, // Success = done.
		codes.InvalidArgument,    // Retrying won't fix an invalid argument.
		codes.PermissionDenied,   // Retrying won't fix a permission denied error.
		codes.Unauthenticated,    // Retrying won't fix an unauthenticated error.
		codes.FailedPrecondition, // Client must wait until the system has been fixed.
		codes.Unimplemented,      // Retrying won't fix this.
		codes.Internal,           // Something is very broken.
		codes.DataLoss,           // Something is very broken.
		codes.AlreadyExists,      // Retrying won't change this.
		codes.Canceled:           // The client canceled: stop the retry loop.
		return false

	// Cases that are debatable:
	case codes.OutOfRange, // Might be retryable if the server does a lot of work.
		codes.NotFound: // Retrying might help, but not rapidly.
		return false

	// Debatable cases. Retry to preserve existing behavior.
	case codes.DeadlineExceeded, // The client or the server expired a timeout.
		codes.ResourceExhausted, // Retry with backoff.
		codes.Unknown:           // Catch-all error code.
		return true

	// Errors that are explicitly retryable:
	case codes.Unavailable, // Client can just retry the call.
		codes.Aborted: // Client can retry the read-modify-write function.
		return true

	// Retry for all unknown errors to preserve existing behavior.
	default:
		return true
	}
}

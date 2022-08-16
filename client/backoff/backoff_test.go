// Copyright 2017 Google LLC. All Rights Reserved.
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

package backoff

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	_ "k8s.io/klog/v2"
)

func TestBackoff(t *testing.T) {
	b := Backoff{
		Min:    time.Duration(1),
		Max:    time.Duration(100),
		Factor: 2,
	}
	for _, test := range []struct {
		b     Backoff
		times int
		want  time.Duration
	}{
		{b, 1, time.Duration(1)},
		{b, 2, time.Duration(2)},
		{b, 3, time.Duration(4)},
		{b, 4, time.Duration(8)},
		{b, 8, time.Duration(100)},
	} {
		test.b.Reset()
		var got time.Duration
		for i := 0; i < test.times; i++ {
			got = test.b.Duration()
		}
		if got != test.want {
			t.Errorf("Duration() %v times: %v, want %v", test.times, got, test.want)
		}
	}
}

func TestJitter(t *testing.T) {
	b := Backoff{
		Min:    1 * time.Second,
		Max:    100 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	for _, test := range []struct {
		b     Backoff
		times int
		min   time.Duration
		max   time.Duration
	}{
		{b, 1, 1 * time.Second, 2 * time.Second},
		{b, 2, 2 * time.Second, 4 * time.Second},
		{b, 3, 4 * time.Second, 8 * time.Second},
		{b, 4, 8 * time.Second, 16 * time.Second},
		{b, 8, 100 * time.Second, 200 * time.Second},
	} {
		test.b.Reset()
		var got1 time.Duration
		for i := 0; i < test.times; i++ {
			got1 = test.b.Duration()
		}
		if got1 < test.min || got1 > test.max {
			t.Errorf("Duration() %v times, want  %v < %v < %v", test.times, test.min, got1, test.max)
		}

		// Ensure a random value is being produced.
		test.b.Reset()
		var got2 time.Duration
		for i := 0; i < test.times; i++ {
			got2 = test.b.Duration()
		}
		if got1 == got2 {
			t.Errorf("Duration() %v times == Duration() %v times, want  %v != %v",
				test.times, test.times, got1, got2)
		}
	}
}

func TestRetry(t *testing.T) {
	b := Backoff{
		Min:    50 * time.Millisecond,
		Max:    200 * time.Millisecond,
		Factor: 2,
	}

	// ctx used by Retry(), declared here to that test.ctxFunc can set it.
	var ctx context.Context
	var cancel context.CancelFunc

	for _, test := range []struct {
		name    string
		f       func() error
		ctxFunc func()
		wantErr bool
	}{
		{
			name: "func that immediately succeeds",
			f:    func() error { return nil },
		},
		{
			name: "func that succeeds on second attempt",
			f: func() func() error {
				var callCount int
				return func() error {
					callCount++
					if callCount == 1 {
						return status.Errorf(codes.Unavailable, "error")
					}
					return nil
				}
			}(),
		},
		{
			name: "explicitly retry",
			f: func() func() error {
				var callCount int
				return func() error {
					callCount++
					if callCount < 10 {
						return RetriableErrorf("attempt %d", callCount)
					}
					return nil
				}
			}(),
		},
		{
			name: "explicitly retry and fail",
			f: func() func() error {
				var callCount int
				return func() error {
					callCount++
					if callCount < 10 {
						return RetriableErrorf("attempt %d", callCount)
					}
					return errors.New("failed 10 times")
				}
			}(),
			wantErr: true,
		},
		{
			name: "func that takes too long to succeed",
			f: func() error {
				// Cancel the context and return an error. This func will succeed on
				// any future calls, but it should not be retried due to the context
				// being cancelled.
				if ctx.Err() == nil {
					cancel()
					return status.Errorf(codes.Unavailable, "error")
				}
				return nil
			},
			wantErr: true,
		},
		{
			name: "context done before Retry() called",
			f: func() error {
				return nil
			},
			ctxFunc: func() {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			},
			wantErr: true,
		},
	} {
		if test.ctxFunc != nil {
			test.ctxFunc()
		} else {
			ctx, cancel = context.WithCancel(context.Background())
		}

		err := b.Retry(ctx, test.f)
		cancel()
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: Retry() = %v, want err? %v", test.name, err, test.wantErr)
		}
	}
}

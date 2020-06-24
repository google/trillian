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

package testonly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election2"
)

// Tests is the full list of available Election tests.
// TODO(pavelkalinnikov): Add tests for unexpected mastership loss.
var Tests = []NamedTest{
	{Name: "RunElectionAwait", Run: runElectionAwait},
	{Name: "RunElectionWithMastership", Run: runElectionWithMastership},
	{Name: "RunElectionResign", Run: runElectionResign},
	{Name: "RunElectionClose", Run: runElectionClose},
	{Name: "RunElectionLoop", Run: runElectionLoop},
}

// NamedTest is a test function paired with its string name.
type NamedTest struct {
	Name string
	Run  func(t *testing.T, f election2.Factory)
}

// checkNotDone ensures that the context is not done for some time.
func checkNotDone(ctx context.Context, t *testing.T) {
	t.Helper()
	if err := clock.SleepContext(ctx, 100*time.Millisecond); err != nil {
		t.Error("unexpected context cancelation")
	}
}

// checkDone ensures that the context is done within the specified duration.
func checkDone(ctx context.Context, t *testing.T, wait time.Duration) {
	t.Helper()
	if wait == 0 {
		select {
		case <-ctx.Done(): // Ok.
		default:
			t.Error("expected context done")
		}
	} else if err := clock.SleepContext(ctx, wait); err == nil {
		t.Errorf("expected context done within %v", wait)
	}
}

// runElectionAwait tests the Await call with different pre-conditions.
func runElectionAwait(t *testing.T, f election2.Factory) {
	awaitErr := errors.New("await error")
	for _, tc := range []struct {
		desc    string
		block   bool
		cancel  bool
		err     error
		wantErr error
	}{
		{desc: "ok", wantErr: nil},
		{desc: "cancel", block: true, cancel: true, wantErr: context.Canceled},
		{desc: "deadline", block: true, cancel: false, wantErr: context.DeadlineExceeded},
		{desc: "error", err: awaitErr, wantErr: awaitErr},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			d := NewDecorator(e)
			d.Update(Errs{Await: tc.err})
			d.BlockAwait(tc.block)

			if tc.cancel {
				cancel()
			} else if tc.block {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			if got, want := d.Await(ctx), tc.wantErr; got != want {
				t.Errorf("Await(): %v, want %v", got, want)
			}
		})
	}
}

// runElectionWithMastership tests the WithMastership call.
func runElectionWithMastership(t *testing.T, f election2.Factory) {
	ctxErr := errors.New("WithMastership error")
	for _, tc := range []struct {
		desc     string
		beMaster bool
		cancel   bool
		err      error
		wantErr  error
	}{
		{desc: "master", beMaster: true},
		{desc: "master-cancel", beMaster: true, cancel: true},
		{desc: "not-master", beMaster: false},
		{desc: "not-master-cancel", cancel: true},
		{desc: "error", err: ctxErr, wantErr: ctxErr},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			d := NewDecorator(e)
			d.Update(Errs{WithMastership: tc.err})
			if tc.beMaster {
				if err := d.Await(ctx); err != nil {
					t.Fatalf("Await(): %v", err)
				}
			}
			mctx, err := d.WithMastership(ctx)
			if want := tc.wantErr; err != want {
				t.Fatalf("WithMastership(): %v, want %v", err, want)
			}
			if err != nil {
				return
			}
			if tc.cancel {
				cancel()
			}
			if tc.beMaster && !tc.cancel {
				checkNotDone(mctx, t)
			} else {
				checkDone(mctx, t, 0)
			}
		})
	}
}

// runElectionResign tests the Resign call.
func runElectionResign(t *testing.T, f election2.Factory) {
	resignErr := errors.New("resign error")
	for _, tc := range []struct {
		desc     string
		beMaster bool
		err      error
		wantErr  error
	}{
		{desc: "master", beMaster: true, wantErr: nil},
		{desc: "master-error", beMaster: true, err: resignErr, wantErr: resignErr},
		{desc: "not-master", beMaster: false, wantErr: nil},
		{desc: "not-master-error", err: resignErr, wantErr: resignErr},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			d := NewDecorator(e)
			d.Update(Errs{Resign: tc.err})

			if tc.beMaster {
				if err := d.Await(ctx); err != nil {
					t.Fatalf("Await(): %v", err)
				}
			}
			mctx, err := d.WithMastership(ctx)
			if err != nil {
				t.Fatalf("WithMastership(): %v", err)
			}
			if got, want := d.Resign(ctx), tc.wantErr; got != want {
				t.Errorf("Resign(): %v, want %v", got, want)
			}
			if tc.beMaster && tc.wantErr == nil {
				checkDone(mctx, t, 1*time.Second)
			} else if tc.beMaster {
				checkNotDone(mctx, t)
			} else {
				checkDone(mctx, t, 0)
			}
		})
	}
}

// runElectionClose tests the Close call.
func runElectionClose(t *testing.T, f election2.Factory) {
	closeErr := errors.New("close error")
	for _, tc := range []struct {
		desc     string
		beMaster bool
		cancel   bool
		err      error
		wantErr  error
	}{
		{desc: "master", beMaster: true, wantErr: nil},
		{desc: "master-error", beMaster: true, err: closeErr, wantErr: closeErr},
		{desc: "not-master", beMaster: false, wantErr: nil},
		{desc: "not-master-error", err: closeErr, wantErr: closeErr},
		// TODO(pavelkalinnikov): reinstate when not flaky.
		// {desc: "cancel", cancel: true, wantErr: nil},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			d := NewDecorator(e)
			d.Update(Errs{Close: tc.err})

			if tc.beMaster {
				if err := d.Await(ctx); err != nil {
					t.Fatalf("Await(): %v", err)
				}
			}
			mctx, err := d.WithMastership(ctx)
			if err != nil {
				t.Fatalf("WithMastership(): %v", err)
			}

			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if tc.cancel {
				cancel()
			}
			if got, want := d.Close(cctx), tc.wantErr; got != want {
				t.Errorf("Close(): %v, want %v", got, want)
			}
			if tc.beMaster && tc.wantErr == nil {
				checkDone(mctx, t, 1*time.Second)
			} else if tc.beMaster {
				checkNotDone(mctx, t)
			} else {
				checkDone(mctx, t, 0)
			}
		})
	}
}

// runElectionLoop runs a typical mastership loop.
func runElectionLoop(t *testing.T, f election2.Factory) {
	ctx := context.Background()
	e, err := f.NewElection(ctx, "testID")
	if err != nil {
		t.Fatalf("NewElection(): %v", err)
	}
	d := NewDecorator(e)

	defer func() {
		if err := d.Close(ctx); err != nil {
			t.Errorf("Close(): %v", err)
		}
	}()

	for i := 0; i < 10; i++ {
		t.Logf("Mastership iteration: %d", i)
		if err := d.Await(ctx); err != nil {
			t.Fatalf("Await(): %v", err)
		}
		mctx, err := d.WithMastership(ctx)
		if err != nil {
			t.Fatalf("WithMastership(): %v", err)
		}
		checkNotDone(mctx, t) // Do some work as master.
		if err := d.Resign(ctx); err != nil {
			t.Errorf("Resign(): %v", err)
		}
		checkDone(mctx, t, 1*time.Second) // The mastership context should close.
	}
}

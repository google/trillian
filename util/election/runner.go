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

package election

import (
	"context"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/util"
)

// Config is a set of parameters for a continuous master election process.
type Config struct {
	// PreElectionPause is the maximum interval to wait before starting election.
	PreElectionPause time.Duration
	// MasterCheckInterval is the interval between checks that the instance still
	// holds mastership.
	MasterCheckInterval time.Duration
	// TODO(pavelkalinnikov): Add TTL to tolerate temporary IsMaster hiccups.
}

// Runner coordinates election participation for a single instance.
//
// An example usage pattern:
//
//   runner := NewRunner(...)
//   defer runner.Close(ctx)
//
//   beMaster := func(ctx context.Context) error {
//     if err := runner.Start(ctx); err != nil {
//       return err
//     }
//     mctx = runner.MasterContext()
//     defer runner.Stop()
//
//     for { // While we are the master.
//       // Note: The work will be canceled if mastership is suddenly over.
//       if err := DoSomeWorkAsMaster(mctx); err != nil {
//         // Return if mctx is done, or report err and retry if possible.
//       }
//       if ShouldResign() {
//         return runner.Resign(ctx)
//       }
//     }
//   }
//
//   for !ShouldStop() { // Be the master potentially multiple times.
//     if err := beMaster(ctx); err != nil {
//       // Return if ctx canceled, or report the error and retry if possible.
//     }
//  }
type Runner struct {
	cfg    *Config
	me     MasterElection
	prefix string // Prefix for all glog messages.

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRunner returns a new election runner for the provided configuration and
// MasterElection interface. After using the Runner, the caller must invoke
// Close to release resources.
func NewRunner(cfg *Config, me MasterElection, ts util.TimeSource, prefix string) *Runner {
	return &Runner{cfg: cfg, me: me, prefix: prefix}
}

// MasterContext returns the context which is active while the instance
// believes to be the master. Must not be used before first Start.
func (r *Runner) MasterContext() context.Context {
	return r.ctx
}

// Start blocks until it captures mastership on behalf of the current instance.
// Returns an error if capturing fails or the passed in context is canceled
// before that. Effectively does nothing if called after another Start without
// the corresponding Stop or Resign. Must not be called after Close.
//
// The method kicks off a goroutine which tracks mastership status, and
// terminates as soon as mastership is lost, IsMaster check returns an error,
// or the passed in context is canceled. The goroutine maintains MasterContext
// open until it terminates.
func (r *Runner) Start(ctx context.Context) error {
	if r.done != nil {
		return ctx.Err()
	}

	// Pause for a random interval so that if multiple instances start at the
	// same time there is less of a thundering herd.
	pause := rand.Int63n(r.cfg.PreElectionPause.Nanoseconds())
	if err := util.SleepContext(ctx, time.Duration(pause)); err != nil {
		return err
	}

	if err := r.me.Start(ctx); err != nil {
		return err
	}
	if err := r.me.WaitForMastership(ctx); err != nil {
		return err
	}

	done := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)
	r.ctx, r.cancel, r.done = ctx, cancel, done
	go func() {
		defer close(done)
		defer cancel()

		for {
			if err := util.SleepContext(ctx, r.cfg.MasterCheckInterval); err != nil {
				return
			}
			isMaster, err := r.me.IsMaster(ctx)
			if err != nil {
				glog.Warningf("%sIsMaster: %v", r.prefix, err)
				return
			} else if !isMaster {
				glog.Warningf("%sIsMaster: false", r.prefix)
				return
			}
		}
	}()

	return nil
}

// Wait blocks until the Runner is Stopped and released its resources.
func (r *Runner) Wait() {
	<-r.done
}

// Stop shuts down mastership monitoring for this Runner. Returns when the
// resources are released.
func (r *Runner) stop() {
	if r.done != nil {
		r.cancel()
		<-r.done // Terminates assuming MasterElection respects context cancelation.
		r.cancel, r.done = nil, nil
		// Note: MasterContext() remains valid, but Done.
	}
}

// Resign releases mastership for this instance. The Runner is still able to
// participate in new election rounds after.
func (r *Runner) Resign(ctx context.Context) error {
	r.stop()
	return r.me.Resign(ctx)
}

// Close permanently stops this Runner from participating in election.
func (r *Runner) Close(ctx context.Context) error {
	r.stop()
	return r.me.Close(ctx)
}

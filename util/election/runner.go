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
	// TTL is the time after which the instance will resign if encountered no
	// successful IsMaster calls.
	TTL time.Duration
}

// Run contains the status of a single instance's mastership round.
type Run struct {
	// Ctx is the context which is active while the instance believes to be the
	// master, and continues checking its mastership status.
	Ctx context.Context
	// Cancel cancels Ctx, instructs the instance stop checking its mastership
	// status, and commences resource release.
	Cancel context.CancelFunc
	// Done is closed when the resources of this Run are released, and the
	// instance no longer checks its mastership status. After Done is a safe
	// place to invoke Runner.Resign method.
	Done <-chan struct{}
}

// Stop stops this Run checking its mastership status, blocks until its
// resources are released.
func (r *Run) Stop() {
	r.Cancel()
	<-r.Done
}

// Runner coordinates election participation for a single instance.
//
// An example usage pattern:
//
//   defer runner.Close(ctx)
//   for { // We want to be the master potentially multiple times.
//     run, err := runner.AwaitMastership(ctx)
//     if err != nil {
//       // Return if ctx canceled, or retry.
//     }
//     for { // Now we are the master.
//       // Note: The work will be canceled if mastership is suddenly over.
//       if err := DoSomeWorkAsMaster(run.Ctx); err != nil {
//         // ... log error ...
//         if run.Ctx.Err() != nil { // Mastership interrupted.
//           break // Stop doing work as master.
//         }
//         // ... some other error, react accordingly ...
//       }
//       if ShouldResign() {
//         run.Stop()
//         runner.Resign(ctx)
//         break
//       }
//     }
//  }
type Runner struct {
	cfg        *Config
	me         MasterElection
	timeSource util.TimeSource // Allows for mocking tests.
	prefix     string          // Prefix for all glog messages.
}

// NewRunner returns a new election runner for the provided configuration and
// MasterElection interface.
func NewRunner(cfg *Config, me MasterElection, ts util.TimeSource, prefix string) *Runner {
	return &Runner{cfg: cfg, me: me, timeSource: ts, prefix: prefix}
}

// AwaitMastership blocks until it captures mastership on behalf of the current
// instance, and returns a Run object which can be used to observe the
// mastership status. Returns an error if capturing fails or the passed in
// context is canceled before that.
//
// The method kicks off a goroutine which tracks mastership status, and
// terminates as soon as mastership is lost, IsMaster check consistently
// returns an error for longer than TTL, the passed in context is
// canceled, or the returned Run's Ctx is canceled. The goroutine will attempt
// to explicitly Resign when it terminates.
func (r *Runner) AwaitMastership(ctx context.Context) (*Run, error) {
	// Pause for a random interval so that if multiple instances start at the
	// same time there is less of a thundering herd.
	pause := rand.Int63n(r.cfg.PreElectionPause.Nanoseconds())
	if err := util.SleepContext(ctx, time.Duration(pause)); err != nil {
		return nil, err
	}

	if err := r.me.Start(ctx); err != nil {
		return nil, err
	}
	if err := r.me.WaitForMastership(ctx); err != nil {
		return nil, err
	}

	cctx, cancel := context.WithCancel(ctx)
	elected := r.timeSource.Now()
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer cancel()

		for deadline := elected.Add(r.cfg.TTL); ; {
			if err := util.SleepContext(cctx, r.cfg.MasterCheckInterval); err != nil {
				return
			}
			isMaster, err := r.me.IsMaster(cctx)
			now := r.timeSource.Now()
			switch {
			case err != nil:
				glog.Warningf("%sIsMaster: %v", r.prefix, err)
				if cctx.Err() != nil || now.After(deadline) {
					return
				}
			case !isMaster:
				glog.Warningf("%sIsMaster: false", r.prefix)
				return
			default: // We are the master, so update the deadline.
				deadline = now.Add(r.cfg.TTL)
			}
		}
	}()

	return &Run{Ctx: cctx, Cancel: cancel, Done: done}, nil
}

// Resign releases mastership for this instance. The Runner is still able to
// participate in new election rounds after. Before calling this the client
// must wait until mastership monitoring is finished, i.e. Run.Done is closed.
func (r *Runner) Resign(ctx context.Context) error {
	return r.me.Resign(ctx)
}

// Close permanently stops this Runner from participating in election. Before
// calling this the client must wait until mastership monitoring is finished,
// i.e. Run.Done is closed.
func (r *Runner) Close(ctx context.Context) error {
	return r.me.Close(ctx)
}

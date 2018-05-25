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
	// MasterTTL is the time after which the instance will resign if encountered
	// no successful IsMaster calls.
	MasterTTL time.Duration
}

// Run contains mastership status of a single instance during a single run of
// being master.
type Run struct {
	// Start is the time when the instance became the master as measured by the
	// corresponding Runner's TimeSource.
	Start time.Time
	// Ctx is the context which is active while the instance believes to be the
	// master. As soon as Ctx is canceled, the instance attempts to resign, and
	// stops monitoring its status.
	Ctx context.Context
	// Cancel cancels Ctx, or does nothing if that is already canceled.
	// Effectively, it instructs the instance to resign if not yet.
	Cancel context.CancelFunc
	// Done closes when mastership status monitoring goroutine for this run
	// terminates, strictly after Ctx.Done() closes. After calling Cancel, a
	// closed Done indicates completion of resigning (unless it failed, or Ctx
	// parent context was canceled earlier).
	Done <-chan struct{}
}

// Resign triggers mastership resignation, and returns when it is believed to
// be completed.
func (r *Run) Resign() {
	r.Cancel()
	<-r.Done
}

// Runner coordinates election participation for a single instance.
//
// An example usage pattern:
//
//   defer runner.Close(ctx)
//   for { // We want to be the master potentially multiple times.
//     run, err := runner.BeMaster(ctx)
//     if err != nil {
//       // Return if ctx canceled, or retry after some time.Sleep.
//     }
//     for { // Now we are the master.
//       // Note: The work will be canceled if mastership is suddenly over.
//       if err := DoSomeWorkAsMaster(run.Ctx); err != nil {
//         if run.Ctx.Err() == err { // ctx or run.Ctx was canceled.
//           break // Stop doing work as master.
//         }
//         // ... some other error, react accordingly ...
//       }
//       if ShouldResign(run.Start) {
//         run.Resign()
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

// BeMaster blocks until it captures mastership on behalf of the current
// instance, and returns a Run object which can be used to observe the
// mastership status. Returns an error if capturing fails or the passed in
// context is canceled before that.
//
// The method kicks off a goroutine which tracks mastership status, and
// terminates as soon as mastership is lost, IsMaster check consistently
// returns an error for longer than MasterTTL, the passed in context is
// canceled, or the returned Run's Ctx is canceled. The goroutine will attempt
// to explicitly Resign when it terminates.
func (r *Runner) BeMaster(ctx context.Context) (*Run, error) {
	if err := util.SleepContext(ctx, r.cfg.PreElectionPause); err != nil {
		return nil, err
	}
	if err := r.me.Start(ctx); err != nil {
		return nil, err
	}
	if err := r.me.WaitForMastership(ctx); err != nil {
		return nil, err
	}

	elected := r.timeSource.Now()
	cctx, cancel := context.WithCancel(ctx)
	ch := make(chan struct{})

	go func() {
		defer func() {
			// Note: Use parent context to be able to resign if canceling the child.
			if err := r.me.Resign(ctx); err != nil {
				glog.Errorf("%sResign: %v", r.prefix, err)
			}
			cancel()
			close(ch)
		}()

		for deadline := elected.Add(r.cfg.MasterTTL); ; {
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
				deadline = now.Add(r.cfg.MasterTTL)
			}
		}
	}()

	return &Run{Start: elected, Ctx: cctx, Cancel: cancel, Done: ch}, nil
}

// Close permanently stops this Runner from participating in election. Must be
// called after all Runs returned by this Runner are Done.
func (r *Runner) Close(ctx context.Context) error {
	return r.me.Close(ctx)
}

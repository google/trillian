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

// EventType is a type of event occurred to an election instance.
type EventType byte

const (
	BecomeMaster     EventType = iota // The instance believes to be the master.
	NotMaster                         // Lost mastership along the way.
	NotMasterResign                   // Voluntarily resigned from mastership.
	NotMasterTimeout                  // Resigned due to status update timeout.
)

// Config is a set of parameters for a continuous master election process.
type Config struct {
	// PreElectionPause is the maximum interval to wait before starting election.
	PreElectionPause time.Duration
	// RestartInterval is the time the instance will wait before attempting to
	// Start election after an error returned by the previous Start call.
	RestartInterval time.Duration
	// MasterCheckInterval is the interval between checks that the instance still
	// holds mastership.
	MasterCheckInterval time.Duration
	// MasterTTL is the time after which the instance will resign if encountered
	// no successful IsMaster calls.
	MasterTTL time.Duration
	// MasterHoldInterval is the minimum interval to hold mastership for during
	// flawless operation.
	MasterHoldInterval time.Duration
	// ResignOdds gives the chance of resigning mastership after each check
	// interval, as the N for 1-in-N.
	ResignOdds int
}

// ShouldResign returns whether the instance can voluntarily give up on
// mastership according to the config.
func (c *Config) ShouldResign(elected, now time.Time) bool {
	passed := now.Sub(elected)
	if passed < c.MasterHoldInterval {
		// Always hold onto mastership for a minimum interval to prevent churn.
		return false
	}
	// Roll the bones.
	return c.ResignOdds <= 0 || rand.Intn(c.ResignOdds) == 0
}

// Event describes whether the instance has become or stopped being the master.
// Each Event *must* be Ack-ed when acted upon by the subscriber.
type Event struct {
	// Type denotes the type of event occurred to the election instance.
	Type EventType
	// done notifies the Runner back when the Event has been acted upon.
	done chan struct{}
}

// Ack notifies the Runner that it can follow up with a new election round.
// This allows the downstream client act on the fact of resigning before the
// instance re-enters election, e.g. they may want to gracefully/forcefully
// finish the outstanding work.
func (e *Event) Ack() {
	if e.done != nil {
		close(e.done)
	}
}

// Runner coordinates master election for a single instance. Reports mastership
// updates via an Event channel.
type Runner struct {
	cfg        *Config
	me         MasterElection
	timeSource util.TimeSource // Allows for mocking tests.
	prefix     string          // Prefix for all glog messages.
	evts       chan Event
}

// NewRunner returns a new election runner for the provided configuration and
// MasterElection interface.
func NewRunner(cfg *Config, me MasterElection, ts util.TimeSource, prefix string) *Runner {
	return &Runner{cfg: cfg, me: me, timeSource: ts, prefix: prefix}
}

// Run launches a continuous master election process, and returns a channel to
// which it reports updates on this instance's mastership status. Events passed
// to the channel will alternate between becoming and stop being the master
// (see comment above the unexported run method for more details).
//
// The underlying goroutine will stop only if the context is done, in which
// case it will also close the output Event channel.
//
// If the master election process encounters any error other than canceling the
// context, it will put it to glog and try to continue safely.
func (r *Runner) Run(ctx context.Context) <-chan Event {
	if r.evts != nil {
		panic("Run must be invoked only once")
	}
	r.evts = make(chan Event)
	go func() {
		defer close(r.evts)
		if err := r.run(ctx); err != nil {
			glog.Warningf("%selection runner exited: %v", r.prefix, err)
		}
	}()
	return r.evts
}

// run executes election maitaining loop continuously until the passed in
// context is done. Returns the latest error.
//
// Whenever mastership status of the log gets updated, a new Event is sent to
// the output channel. It is guaranteed that the first event (if any) will be
// becoming the master, and the events will alternate between becoming and stop
// being the master (voluntarily, i.e. after MasterHoldInterval passes, or
// unexpectedly, e.g. due to network partitioning).
func (r *Runner) run(ctx context.Context) (err error) {
	// Pause for a random interval so that if multiple instances start at the
	// same time there is less of a thundering herd.
	pause := rand.Int63n(r.cfg.PreElectionPause.Nanoseconds())
	if err := sleepContext(ctx, time.Duration(pause)); err != nil {
		return err
	}

	for {
		if err = r.me.Start(ctx); err == nil {
			break
		}
		glog.Warningf("%sMasterElection.Start: %v", r.prefix, err)
		if err = sleepContext(ctx, r.cfg.RestartInterval); err != nil {
			return err
		}
	}
	defer func(ctx context.Context) {
		if cerr := r.me.Close(ctx); cerr != nil {
			err = cerr
		}
	}(ctx)

	for {
		if err := r.beMaster(ctx); err != nil {
			// TODO(pavelkalinnikov): If WaitForMastership returns errors constantly,
			// this loop is very busy.
			if ctx.Err() != nil {
				return err
			}
			glog.Warningf("%sbeMaster: %v", r.prefix, err)
		}
	}
}

func (r *Runner) beMaster(ctx context.Context) (err error) {
	if err := r.me.WaitForMastership(ctx); err != nil {
		return err
	}

	elected := r.timeSource.Now()
	select {
	case r.evts <- Event{Type: BecomeMaster}:
	case <-ctx.Done():
		return ctx.Err()
	}

	for deadline := elected.Add(r.cfg.MasterTTL); ; {
		if err = sleepContext(ctx, r.cfg.MasterCheckInterval); err != nil {
			return err
		}
		isMaster, err := r.me.IsMaster(ctx)
		now := r.timeSource.Now()
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			glog.Warningf("%sIsMaster: %v", r.prefix, err)
			if now.After(deadline) {
				return r.resign(ctx, NotMasterTimeout)
			}
		case !isMaster:
			return r.resign(ctx, NotMaster)
		case r.cfg.ShouldResign(elected, r.timeSource.Now()):
			return r.resign(ctx, NotMasterResign)
		default: // We are the master, so update the deadline and continue.
			deadline = now.Add(r.cfg.MasterTTL)
		}
	}
}

func (r *Runner) resign(ctx context.Context, typ EventType) error {
	evt := Event{Type: typ, done: make(chan struct{})}
	select {
	case r.evts <- evt:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-evt.done: // Wait until the event is Ack-ed.
	case <-ctx.Done():
		return ctx.Err()
	}
	if typ != BecomeMaster && typ != NotMaster {
		return r.me.ResignAndRestart(ctx)
	}
	return nil
}

// sleepContext sleeps for at least the specified duration. Returns an error
// iff the context is done before the timer fires.
func sleepContext(ctx context.Context, dur time.Duration) error {
	timer := time.NewTimer(dur)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	}
	return nil
}

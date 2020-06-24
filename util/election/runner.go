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

package election

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election2"
)

// Minimum values for configuration intervals.
// TODO(pavelkalinnikov): These parameters are specific to the application, so
// shouldn't be here.
const (
	MinPreElectionPause   = 10 * time.Millisecond
	MinMasterHoldInterval = 10 * time.Second
)

// RunnerConfig describes the parameters for an election Runner.
type RunnerConfig struct {
	// PreElectionPause is the maximum interval to wait before starting a
	// mastership election for a particular log.
	PreElectionPause time.Duration
	// MasterHoldInterval is the minimum interval to hold mastership for.
	MasterHoldInterval time.Duration
	// MasterHoldJitter is the maximum addition to MasterHoldInterval.
	MasterHoldJitter time.Duration

	TimeSource clock.TimeSource
}

// ResignDelay returns a randomized delay of how long to keep mastership for.
func (cfg *RunnerConfig) ResignDelay() time.Duration {
	delay := cfg.MasterHoldInterval
	if cfg.MasterHoldJitter <= 0 {
		return delay
	}
	add := rand.Int63n(int64(cfg.MasterHoldJitter))
	return delay + time.Duration(add)
}

// fixupRunnerConfig ensures operation parameters have required minimum values.
func fixupRunnerConfig(cfg *RunnerConfig) {
	if cfg.PreElectionPause < MinPreElectionPause {
		cfg.PreElectionPause = MinPreElectionPause
	}
	if cfg.MasterHoldInterval < MinMasterHoldInterval {
		cfg.MasterHoldInterval = MinMasterHoldInterval
	}
	if cfg.MasterHoldJitter < 0 {
		cfg.MasterHoldJitter = 0
	}
	if cfg.TimeSource == nil {
		cfg.TimeSource = clock.System
	}
}

// Runner controls a continuous election process.
type Runner struct {
	// Allow the user to store a Cancel function with the runner for convenience.
	Cancel   context.CancelFunc
	id       string
	cfg      *RunnerConfig
	tracker  *MasterTracker
	election election2.Election
}

// NewRunner builds a new election Runner instance with the given configuration.  On calling
// Run(), the provided election will be continuously monitored and mastership changes will
// be notified to the provided MasterTracker instance.
func NewRunner(id string, cfg *RunnerConfig, tracker *MasterTracker, cancel context.CancelFunc, el election2.Election) *Runner {
	fixupRunnerConfig(cfg)
	return &Runner{
		Cancel:   cancel,
		id:       id,
		cfg:      cfg,
		tracker:  tracker,
		election: el,
	}
}

// Run performs a continuous election process. It runs continuously until the
// context is canceled or an internal error is encountered.
func (er *Runner) Run(ctx context.Context, pending chan<- Resignation) {
	// Pause for a random interval so that if multiple instances start at the
	// same time there is less of a thundering herd.
	pause := rand.Int63n(er.cfg.PreElectionPause.Nanoseconds())
	if err := clock.SleepSource(ctx, time.Duration(pause), er.cfg.TimeSource); err != nil {
		return // The context has been canceled during the sleep.
	}

	glog.V(1).Infof("%s: start election-monitoring loop ", er.id)
	defer func() {
		glog.Infof("%s: shutdown election-monitoring loop", er.id)
		if err := er.election.Close(ctx); err != nil {
			glog.Warningf("%s: election.Close: %v", er.id, err)
		}
	}()

	for {
		if err := er.beMaster(ctx, pending); err != nil {
			glog.Errorf("%s: %v", er.id, err)
			break
		}
	}
}

func (er *Runner) beMaster(ctx context.Context, pending chan<- Resignation) error {
	glog.V(1).Infof("%s: When I left you, I was but the learner", er.id)
	if err := er.election.Await(ctx); err != nil {
		return fmt.Errorf("election.Await() failed: %v", err)
	}
	glog.Infof("%s: Now, I am the master", er.id)
	er.tracker.Set(er.id, true)
	defer er.tracker.Set(er.id, false)

	mctx, err := er.election.WithMastership(ctx)
	if err != nil {
		return fmt.Errorf("election.WithMastership() failed: %v", err)
	}

	timer := er.cfg.TimeSource.NewTimer(er.cfg.ResignDelay())
	defer timer.Stop()

	select {
	case <-mctx.Done(): // Mastership context is canceled.
		glog.Errorf("%s: no longer the master!", er.id)
		return mctx.Err()

	case <-timer.Chan():
		glog.Infof("%s: queue up resignation of mastership", er.id)
		done := make(chan struct{})
		r := Resignation{ID: er.id, er: er, done: done}
		select {
		case pending <- r:
			<-done // Block until acted on.
		default:
			glog.Warning("Dropping resignation because operation manager seems to be exiting")
		}
	}
	return nil
}

// Resignation indicates that a master should explicitly resign mastership, by invoking
// the Execute() method at a point where no master-related activity is ongoing.
type Resignation struct {
	ID   string
	er   *Runner
	done chan<- struct{}
}

// Execute performs the pending deliberate resignation for an election runner.
func (r *Resignation) Execute(ctx context.Context) {
	defer close(r.done)
	glog.Infof("%s: deliberately resigning mastership", r.er.id)
	if err := r.er.election.Resign(ctx); err != nil {
		glog.Errorf("%s: failed to resign mastership: %v", r.er.id, err)
	}
}

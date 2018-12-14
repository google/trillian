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

package election

import (
	"context"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/util/clock"
)

// Minimum values for configuration intervals.
const (
	MinPreElectionPause    = 10 * time.Millisecond
	MinMasterCheckInterval = 50 * time.Millisecond
	MinMasterHoldInterval  = 10 * time.Second
)

// RunnerConfig describes the parameters for an election Runner.
type RunnerConfig struct {
	// PreElectionPause is the maximum interval to wait before starting a
	// mastership election for a particular log.
	PreElectionPause time.Duration
	// MasterCheckInterval is the interval between checks that we still
	// hold mastership for a log.
	MasterCheckInterval time.Duration
	// MasterHoldInterval is the minimum interval to hold mastership for.
	MasterHoldInterval time.Duration
	// ResignOdds gives the chance of resigning mastership after each
	// check interval, as the N for 1-in-N.
	ResignOdds int

	TimeSource clock.TimeSource
}

// fixupRunnerConfig ensures operation parameters have required minimum values.
func fixupRunnerConfig(cfg *RunnerConfig) {
	if cfg.PreElectionPause < MinPreElectionPause {
		cfg.PreElectionPause = MinPreElectionPause
	}
	if cfg.MasterCheckInterval < MinMasterCheckInterval {
		cfg.MasterCheckInterval = MinMasterCheckInterval
	}
	if cfg.MasterHoldInterval < MinMasterHoldInterval {
		cfg.MasterHoldInterval = MinMasterHoldInterval
	}
	if cfg.ResignOdds < 1 {
		cfg.ResignOdds = 1
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
	election MasterElection
}

// NewRunner builds a new election Runner instance with the given configuration.  On calling
// Run(), the provided election will be continuously monitored and mastership changes will
// be notified to the provided MasterTracker instance.
func NewRunner(id string, cfg *RunnerConfig, tracker *MasterTracker, cancel context.CancelFunc, el MasterElection) *Runner {
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
	// Pause for a random interval so that if multiple instances start at the same
	// time there is less of a thundering herd.
	pause := rand.Int63n(er.cfg.PreElectionPause.Nanoseconds())
	if err := clock.SleepContext(ctx, time.Duration(pause)); err != nil {
		return
	}

	glog.V(1).Infof("%s: start election-monitoring loop ", er.id)
	if err := er.election.Start(ctx); err != nil {
		glog.Errorf("%s: election.Start() failed: %v", er.id, err)
		return
	}
	defer func(ctx context.Context, er *Runner) {
		glog.Infof("%s: shutdown election-monitoring loop", er.id)
		er.election.Close(ctx)
	}(ctx, er)

	for {
		glog.V(1).Infof("%s: When I left you, I was but the learner", er.id)
		if err := er.election.WaitForMastership(ctx); err != nil {
			glog.Errorf("%s: er.election.WaitForMastership() failed: %v", er.id, err)
			return
		}
		glog.V(1).Infof("%s: Now, I am the master", er.id)
		er.tracker.Set(er.id, true)
		masterSince := er.cfg.TimeSource.Now()

		// While-master loop
		for {
			if err := clock.SleepContext(ctx, er.cfg.MasterCheckInterval); err != nil {
				glog.Infof("%s: termination requested", er.id)
				return
			}
			master, err := er.election.IsMaster(ctx)
			if err != nil {
				glog.Errorf("%s: failed to check mastership status", er.id)
				break
			}
			if !master {
				glog.Errorf("%s: no longer the master!", er.id)
				er.tracker.Set(er.id, false)
				break
			}
			if er.ShouldResign(masterSince) {
				glog.Infof("%s: queue up resignation of mastership", er.id)
				er.tracker.Set(er.id, false)

				done := make(chan bool)
				r := Resignation{ID: er.id, er: er, done: done}
				pending <- r
				<-done // block until acted on
				break  // no longer master
			}
		}
	}
}

// ShouldResign randomly decides whether this runner should resign mastership.
func (er *Runner) ShouldResign(masterSince time.Time) bool {
	now := er.cfg.TimeSource.Now()
	duration := now.Sub(masterSince)
	if duration < er.cfg.MasterHoldInterval {
		// Always hold onto mastership for a minimum interval to prevent churn.
		return false
	}
	// Roll the bones.
	odds := er.cfg.ResignOdds
	if odds <= 0 {
		return true
	}
	return rand.Intn(er.cfg.ResignOdds) == 0
}

// Resignation indicates that a master should explicitly resign mastership, by invoking
// the Execute() method at a point where no master-related activity is ongoing.
type Resignation struct {
	ID   string
	er   *Runner
	done chan<- bool
}

// Execute performs the pending deliberate resignation for an election runner.
func (r *Resignation) Execute(ctx context.Context) {
	glog.Infof("%s: deliberately resigning mastership", r.er.id)
	if err := r.er.election.Resign(ctx); err != nil {
		glog.Errorf("%s: failed to resign mastership: %v", r.er.id, err)
	}
	if err := r.er.election.Start(ctx); err != nil {
		glog.Errorf("%s: failed to restart election: %v", r.er.id, err)
	}
	r.done <- true
}

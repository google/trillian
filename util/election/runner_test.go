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

package election_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election/stub"
)

func TestShouldResign(t *testing.T) {
	startTime := time.Date(1970, 9, 19, 12, 00, 00, 00, time.UTC)
	var tests = []struct {
		hold     time.Duration
		odds     int
		wantHold time.Duration
		wantProb float64
	}{
		{hold: 12 * time.Second, odds: 10, wantHold: 12 * time.Second, wantProb: 0.1},
		{hold: time.Second, odds: 10, wantHold: 10 * time.Second, wantProb: 0.1},
		{hold: 10 * time.Second, odds: 0, wantHold: 10 * time.Second, wantProb: 1.0},
		{hold: 10 * time.Second, odds: 1, wantHold: 10 * time.Second, wantProb: 1.0},
		{hold: 10 * time.Second, odds: -1, wantHold: 10 * time.Second, wantProb: 1.0},
	}
	for _, test := range tests {
		fakeTimeSource := clock.NewFake(time.Time{})
		cfg := election.RunnerConfig{
			MasterHoldInterval: test.hold,
			ResignOdds:         test.odds,
			TimeSource:         fakeTimeSource,
		}
		er := election.NewRunner("6962", &cfg, nil, nil, nil)

		holdChecks := int64(10)
		holdNanos := test.wantHold.Nanoseconds() / holdChecks
	timeslot:
		for i := int64(-1); i < holdChecks; i++ {
			gapNanos := time.Duration(i * holdNanos)
			fakeTimeSource.Set(startTime.Add(gapNanos))
			for j := 0; j < 100; j++ {
				if er.ShouldResign(startTime) {
					t.Errorf("shouldResign(hold=%v,odds=%v @ start+%v)=true; want false", test.hold, test.odds, gapNanos)
					continue timeslot
				}
			}
		}

		fakeTimeSource.Set(startTime.Add(test.wantHold).Add(time.Nanosecond))
		iterations := 10000
		count := 0
		for i := 0; i < iterations; i++ {
			if er.ShouldResign(startTime) {
				count++
			}
		}
		got := float64(count) / float64(iterations)
		deltaFraction := math.Abs(got-test.wantProb) / test.wantProb
		if deltaFraction > 0.05 {
			t.Errorf("P(shouldResign(hold=%v,odds=%v))=%f; want ~%f", test.hold, test.odds, got, test.wantProb)
		}
	}
}

// TODO(pavelkalinnikov): Reduce flakiness risk in this test by making fewer
// time assumptions.
func TestElectionRunnerRun(t *testing.T) {
	fakeTimeSource := clock.NewFake(time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC))
	cfg := election.RunnerConfig{TimeSource: fakeTimeSource}
	var tests = []struct {
		desc       string
		election   *stub.MasterElection
		lostMaster bool
		wantMaster bool
	}{
		// Basic cases
		{
			desc:     "IsNotMaster",
			election: stub.NewMasterElection(false, nil),
		},
		{
			desc:       "IsMaster",
			election:   stub.NewMasterElection(true, nil),
			wantMaster: true,
		},
		{
			desc:       "LostMaster",
			election:   stub.NewMasterElection(true, nil),
			lostMaster: true,
			wantMaster: false,
		},
		// Error cases
		{
			desc: "ErrorOnStart",
			election: stub.NewMasterElection(false,
				&stub.Errors{Start: errors.New("on start")}),
		},
		{
			desc: "ErrorOnWait",
			election: stub.NewMasterElection(false,
				&stub.Errors{Wait: errors.New("on wait")}),
		},
		{
			desc: "ErrorOnIsMaster",
			election: stub.NewMasterElection(true,
				&stub.Errors{IsMaster: errors.New("on IsMaster")}),
			wantMaster: true,
		},
		{
			desc: "ErrorOnResign",
			election: stub.NewMasterElection(true,
				&stub.Errors{Resign: errors.New("resignation failure")}),
			wantMaster: true,
		},
	}
	const logID = "6962"
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var wg sync.WaitGroup
			ctx, cancel := context.WithCancel(context.Background())

			startTime := time.Now()
			fakeTimeSource := clock.NewFake(startTime)

			el := test.election
			tracker := election.NewMasterTracker([]string{logID}, nil)
			er := election.NewRunner(logID, &cfg, tracker, nil, el)
			resignations := make(chan election.Resignation, 100)
			wg.Add(1)
			go func() {
				defer wg.Done()
				er.Run(ctx, resignations)
			}()
			time.Sleep((3 * election.MinPreElectionPause) / 2)
			if test.lostMaster {
				el.Update(false, nil)
			}

			// Advance fake time so that shouldResign() triggers too.
			fakeTimeSource.Set(startTime.Add(24 * 60 * time.Hour))
			time.Sleep(election.MinMasterCheckInterval)
			for len(resignations) > 0 {
				r := <-resignations
				r.Execute(ctx)
			}

			cancel()
			wg.Wait()
			held := tracker.Held()
			if got := (len(held) > 0 && held[0] == logID); got != test.wantMaster {
				t.Errorf("held=%v => master=%v; want %v", held, got, test.wantMaster)
			}
		})
	}
}

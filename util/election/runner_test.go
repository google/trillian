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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election/stub"
)

func TestConfigResignDelay(t *testing.T) {
	const checks = 1000
	for _, tc := range []struct {
		hold    time.Duration
		jitter  time.Duration
		wantMax time.Duration
	}{
		{hold: 12 * time.Second, jitter: -1 * time.Second, wantMax: 12 * time.Second},
		{hold: 12 * time.Second, jitter: 0, wantMax: 12 * time.Second},
		{hold: 12 * time.Second, jitter: 5 * time.Second, wantMax: 17 * time.Second},
		{hold: 0, jitter: 0, wantMax: 0},
		{hold: 0, jitter: 500 * time.Millisecond, wantMax: 500 * time.Millisecond},
		{hold: 1 * time.Hour, jitter: 10 * time.Minute, wantMax: 70 * time.Minute},
	} {
		t.Run(fmt.Sprintf("%v:%v", tc.hold, tc.jitter), func(t *testing.T) {
			cfg := election.RunnerConfig{
				MasterHoldInterval: tc.hold,
				MasterHoldJitter:   tc.jitter,
			}
			for i := 0; i < checks; i++ {
				d := cfg.ResignDelay()
				if min, max := tc.hold, tc.wantMax; d < min || d > max {
					t.Fatalf("ResignDelay(): %v, want between %v and %v", d, min, max)
				}
			}
		})
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

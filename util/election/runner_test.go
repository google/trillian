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
	to "github.com/google/trillian/util/election2/testonly"
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
	const logID = "6962"
	checkMaster := func(t *testing.T, held []string, wantMaster bool) {
		t.Helper()
		if got := len(held) > 0 && held[0] == logID; got != wantMaster {
			t.Errorf("holding: %v: master=%v, want %v", held, got, wantMaster)
		}
	}

	for _, tc := range []struct {
		desc       string
		errs       to.Errs
		isMaster   bool
		wantMaster bool
		loseMaster bool
		resign     bool
	}{
		// Basic cases.
		{desc: "not-master"},
		{desc: "is-master", isMaster: true, wantMaster: true},
		{desc: "lose-master", isMaster: true, wantMaster: true, loseMaster: true},
		{desc: "resign", isMaster: true, wantMaster: true, resign: true},
		// Error cases.
		{desc: "err-await", errs: to.Errs{Await: errors.New("ErrAwait")}},
		{desc: "err-mctx", errs: to.Errs{WithMastership: errors.New("ErrMastership")}},
		{
			desc:     "err-mctx-after-master",
			isMaster: true,
			errs:     to.Errs{WithMastership: errors.New("ErrMastership")},
		},
		{desc: "err-resign", errs: to.Errs{Resign: errors.New("ErrResign")}},
		{
			desc:       "err-resign-after-master",
			isMaster:   true,
			wantMaster: true,
			errs:       to.Errs{Resign: errors.New("ErrResign")},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			e := to.NewElection()
			d := to.NewDecorator(e)
			d.Update(tc.errs)
			d.BlockAwait(!tc.isMaster)

			start := time.Now()
			ts := clock.NewFake(start)
			tracker := election.NewMasterTracker([]string{logID}, nil)
			cfg := election.RunnerConfig{TimeSource: ts}
			er := election.NewRunner(logID, &cfg, tracker, nil, d)
			resignations := make(chan election.Resignation, 100)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				er.Run(ctx, resignations)
			}()

			// TODO(pavelkalinnikov): This Sleep is necessary to let Run create a
			// Timer *before* we modify the time. To reduce risk of flakes,
			// FakeTimeSource should expose timer creation to this test.
			time.Sleep(100 * time.Millisecond)
			checkMaster(t, tracker.Held(), false)

			ts.Set(start.Add(election.MinPreElectionPause))
			time.Sleep(100 * time.Millisecond) // Now it *can* become the master.
			checkMaster(t, tracker.Held(), tc.wantMaster)

			if tc.loseMaster {
				d.BlockAwait(true)
				if err := e.Resign(ctx); err != nil {
					t.Errorf("Lose mastership: %v", err)
				}
				wg.Wait() // Runner exits on unexpected mastership loss.
				checkMaster(t, tracker.Held(), false)
			}

			if tc.resign {
				d.BlockAwait(true)
				// Advance fake time so that resignation triggers too, if still master.
				ts.Set(start.Add(24 * 60 * time.Hour))
				time.Sleep(100 * time.Millisecond)
				for len(resignations) > 0 {
					r := <-resignations
					r.Execute(ctx)
				}
				time.Sleep(100 * time.Millisecond)
			}

			checkMaster(t, tracker.Held(), tc.wantMaster && !tc.loseMaster && !tc.resign)
			cancel()  // If Runner is still running, it should stop now.
			wg.Wait() // Wait until it stops.
			checkMaster(t, tracker.Held(), false)
		})
	}
}

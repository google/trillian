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

package election_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/trillian/util"
	. "github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election/mock"
)

var (
	cfg = Config{
		PreElectionPause:    time.Millisecond,
		RestartInterval:     time.Millisecond,
		MasterCheckInterval: time.Millisecond,
		MasterHoldInterval:  time.Millisecond,
	}
	noEventDur = 10 * time.Millisecond
	mockError  = errors.New("mock.MasterElection error")
)

func expectEvent(t *testing.T, evts <-chan Event, want EventType) Event {
	t.Helper()
	evt, ok := <-evts
	if !ok {
		t.Errorf("Expected %v, but the channel was closed", want)
		return evt
	}
	if got := evt.Type; got != want {
		t.Errorf("Type=%v, want %v", got, want)
	}
	return evt
}

func expectNoEvent(t *testing.T, evts <-chan Event) {
	t.Helper()
	timer := time.NewTimer(noEventDur)
	select {
	case <-timer.C:
	case evt := <-evts:
		timer.Stop()
		t.Errorf("Expected no event, got %+v", evt)
	}
}

func expectClosed(t *testing.T, evts <-chan Event) {
	t.Helper()
	if evt, ok := <-evts; ok {
		t.Errorf("Expected closed channel, got %+v", evt)
	}
}

func waitResigned(em *mock.MasterElection) {
	for {
		if is, _ := em.IsMaster(context.Background()); !is {
			return
		}
		time.Sleep(noEventDur)
	}
}

func TestShouldResign(t *testing.T) {
	startTime := time.Date(1970, 9, 19, 12, 00, 00, 00, time.UTC)
	holdChecks, iterations := int64(10), 10000

	var tests = []struct {
		hold     time.Duration
		odds     int
		overheld time.Duration
		wantProb float64
	}{
		{hold: 12 * time.Second, odds: 10, overheld: 0 * time.Second, wantProb: 0.1},
		{hold: 12 * time.Second, odds: 10, overheld: 1 * time.Second, wantProb: 0.1},
		{hold: 12 * time.Second, odds: 10, overheld: 10 * time.Second, wantProb: 0.1},
		{hold: 12 * time.Second, odds: 10, overheld: -1 * time.Second, wantProb: 0.0},

		{hold: 1 * time.Second, odds: 3, overheld: 0 * time.Second, wantProb: 1.0 / 3},
		{hold: 1 * time.Second, odds: 3, overheld: 10 * time.Second, wantProb: 1.0 / 3},
		{hold: 1 * time.Second, odds: 10, overheld: 10 * time.Second, wantProb: 0.1},
		{hold: 10 * time.Second, odds: 0, overheld: 10 * time.Second, wantProb: 1.0},
		{hold: 10 * time.Second, odds: 1, overheld: 10 * time.Second, wantProb: 1.0},
		{hold: 10 * time.Second, odds: -1, overheld: 10 * time.Second, wantProb: 1.0},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("hold:%v:odds:%v", test.hold, test.odds), func(t *testing.T) {
			cfg := Config{
				MasterHoldInterval: test.hold,
				ResignOdds:         test.odds,
			}
			holdNanos := test.hold.Nanoseconds() / holdChecks
			for i := int64(-1); i < holdChecks; i++ {
				gapNanos := time.Duration(i * holdNanos)
				now := startTime.Add(gapNanos)
				for j := 0; j < 100; j++ {
					if cfg.ShouldResign(startTime, now) {
						t.Fatalf("shouldResign(start+%v)=true, want false", gapNanos)
					}
				}
			}

			now := startTime.Add(test.hold).Add(test.overheld)
			count := 0
			for i := 0; i < iterations; i++ {
				if cfg.ShouldResign(startTime, now) {
					count++
				}
			}
			got := float64(count) / float64(iterations)
			deltaFraction := math.Abs(got-test.wantProb) / test.wantProb
			if deltaFraction > 0.05 {
				t.Errorf("P(shouldResign)=%f; want ~%f", got, test.wantProb)
			}
		})
	}
}

func TestRunner_BecomeMaster(t *testing.T) {
	me, ts := mock.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())

	expectNoEvent(t, evts)
	me.Update(true, nil)
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts) // Still holds mastership.
}

func TestRunner_ForcefullyResignAndRecover(t *testing.T) {
	me, ts := mock.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())
	expectEvent(t, evts, BecomeMaster)

	me.Update(false, nil) // Force losing mastership.
	evt := expectEvent(t, evts, NotMaster)
	expectNoEvent(t, evts)

	me.Update(true, nil)   // Make mastership available again.
	expectNoEvent(t, evts) // But observe no taking over.
	evt.Ack()              // Because need to Ack the event to continue.
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_VoluntarilyResignAndRecover(t *testing.T) {
	ctx := context.Background()

	me, ts := mock.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(ctx)
	expectEvent(t, evts, BecomeMaster)

	// Advance fake time just enough to trigger resignation.
	ts.Set(ts.Now().Add(cfg.MasterHoldInterval))
	evt := expectEvent(t, evts, NotMasterResign)
	expectNoEvent(t, evts) // Should not re-enter election.
	if is, _ := me.IsMaster(ctx); !is {
		t.Errorf("Should not call ResignAndRestart until after Ack")
	}

	me.Update(true, nil)   // Make mastership available again.
	expectNoEvent(t, evts) // But observe no taking over.
	evt.Ack()              // Because need to Ack the event to continue.
	waitResigned(me)       // Which will finalize the resignation.
	expectNoEvent(t, evts) // Still no taking over, because Ack has given it away.
	me.Update(true, nil)   // But it should be taken over when available again.
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_HealthyLoop(t *testing.T) {
	ctx := context.Background()

	me, ts := mock.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(ctx)

	for iter := 0; iter < 10; iter++ {
		me.Update(true, nil)
		expectEvent(t, evts, BecomeMaster)
		ts.Set(ts.Now().Add(cfg.MasterHoldInterval)) // Trigger resign.
		evt := expectEvent(t, evts, NotMasterResign)
		expectNoEvent(t, evts)
		if is, _ := me.IsMaster(ctx); !is {
			t.Errorf("Should not call ResignAndRestart until after Ack")
		}
		evt.Ack()
		waitResigned(me)
		expectNoEvent(t, evts)
	}
}

func TestRunner_ErrorStart(t *testing.T) {
	me := mock.NewMasterElection(false, &mock.Errors{Start: mockError})
	ts := &util.FakeTimeSource{}

	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())
	expectNoEvent(t, evts)

	me.Update(true, nil) // Recover when error disappears.
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_ErrorWaitForMastership(t *testing.T) {
	me := mock.NewMasterElection(false, &mock.Errors{Wait: mockError})
	ts := &util.FakeTimeSource{}

	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())
	expectNoEvent(t, evts)

	me.Update(true, nil) // Recover when error disappears.
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_ErrorIsMaster(t *testing.T) {
	me := mock.NewMasterElection(true, &mock.Errors{IsMaster: mockError})
	ts := &util.FakeTimeSource{}

	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())
	expectEvent(t, evts, BecomeMaster)

	expectNoEvent(t, evts)
	ts.Set(ts.Now().Add(cfg.MasterTTL)) // Right within the TTL.
	expectNoEvent(t, evts)              // Should not trigger resignation.
	ts.Set(ts.Now().Add(1 * time.Nanosecond))
	evt := expectEvent(t, evts, NotMasterTimeout) // But +1 nanosecond should.
	evt.Ack()
	waitResigned(me)

	me.Update(true, nil) // Recover when error disappears.
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_DisconnectAfterStart(t *testing.T) {
	me, ts := mock.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	evts := runner.Run(context.Background())
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)

	me.Update(false, mock.ErrAll(mockError))
	ts.Set(ts.Now().Add(cfg.MasterTTL + 1))
	evt := expectEvent(t, evts, NotMasterTimeout)
	evt.Ack()
	expectNoEvent(t, evts)

	me.Update(true, nil)
	expectEvent(t, evts, BecomeMaster)
	expectNoEvent(t, evts)
}

func TestRunner_CancelBeforeStart(t *testing.T) {
	me, ts := mock.NewMasterElection(false, nil), &util.FakeTimeSource{}

	// Delay election start so that we can intrude with cancel() before it.
	cfg := cfg // Don't spoil the shared config template.
	cfg.PreElectionPause = 100 * time.Hour
	runner := NewRunner(&cfg, me, ts, "")

	ctx, cancel := context.WithCancel(context.Background())
	evts := runner.Run(ctx)
	expectNoEvent(t, evts)
	cancel()
	expectClosed(t, evts)
}

func TestRunner_CancelWhenMaster(t *testing.T) {
	me, ts := mock.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx, cancel := context.WithCancel(context.Background())
	evts := runner.Run(ctx)
	expectEvent(t, evts, BecomeMaster)
	cancel()
	expectClosed(t, evts)
}

func TestRunner_CancelWhenNotMaster(t *testing.T) {
	me, ts := mock.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx, cancel := context.WithCancel(context.Background())
	evts := runner.Run(ctx)
	expectNoEvent(t, evts) // Give it some time, may hit WaitForMastership.
	cancel()
	me.Ping() // Trigger context canceling in WaitForMastership if stuck there.
	expectClosed(t, evts)
}

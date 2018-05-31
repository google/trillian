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
	"testing"
	"time"

	"github.com/google/trillian/util"
	. "github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election/stub"
)

var (
	cfg = Config{
		PreElectionPause:    time.Millisecond,
		MasterCheckInterval: time.Millisecond,
	}
	decentDur = 10 * time.Millisecond
	stubErr   = errors.New("stub.MasterElection error")
)

// expectNotClosed checks that done is not closed within some timeout.
func expectNotClosed(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()
	if !util.Sleep(done, decentDur) {
		t.Errorf(msg)
	}
}

func becomeMaster(ctx context.Context, t *testing.T, me *stub.MasterElection, runner *Runner) (run *Run) {
	isMaster := make(chan struct{})
	var err error
	go func() {
		defer close(isMaster)
		run, err = runner.AwaitMastership(ctx)
	}()
	expectNotClosed(t, isMaster, "Became master unexpectedly")

	me.Update(true, nil)
	<-isMaster // Now should become the master.
	if err != nil {
		t.Fatalf("AwaitMastership(): %v", err)
	}

	if run == nil {
		t.Fatal("Expected non-nil Run")
	}
	expectNotClosed(t, run.Ctx.Done(), "Closed master context unexpectedly")
	expectNotClosed(t, run.Done, "Closed Done unexpectedly")
	if is, _ := me.IsMaster(ctx); !is {
		t.Errorf("Unexpected masteship resignation")
	}

	return run
}

func TestRunner_BecomeMasterAndCancel(t *testing.T) {
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx, cancel := context.WithCancel(context.Background())
	run := becomeMaster(ctx, t, me, runner)
	cancel()
	<-run.Done // Runner should terminate.
	if run.Ctx.Err() == nil {
		t.Error("Expected closed master context")
	}
	if is, _ := me.IsMaster(ctx); !is {
		t.Error("Unexpected masteship resignation")
	}
}

func TestRunner_BecomeMasterAndResign(t *testing.T) {
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx := context.Background()
	run := becomeMaster(ctx, t, me, runner)

	run.Stop()
	if run.Ctx.Err() == nil {
		t.Error("Expected closed master context")
	}
	if is, _ := me.IsMaster(ctx); !is {
		t.Error("Unexpected masteship resignation")
	}

	runner.Resign(ctx)
	if is, _ := me.IsMaster(ctx); is {
		t.Error("Expected masteship resignation")
	}
}

func TestRunner_BecomeMasterAndError(t *testing.T) {
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx := context.Background()
	run := becomeMaster(ctx, t, me, runner)

	me.Update(true, &stub.Errors{IsMaster: stubErr})
	<-run.Done // Mastership monitoring should terminate.

	if is, _ := me.IsMaster(ctx); !is {
		t.Errorf("Unexpected masteship resignation")
	}
}

func TestRunner_HealthyLoop(t *testing.T) {
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		run := becomeMaster(ctx, t, me, runner)
		// ... do some work as master ...
		run.Stop()
		if run.Ctx.Err() == nil {
			t.Error("Expected closed context")
		}
		runner.Resign(ctx)
		if is, _ := me.IsMaster(ctx); is {
			t.Errorf("Expected masteship resignation")
		}
	}

	if err := runner.Close(ctx); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

func TestRunner_AwaitMastershipErrors(t *testing.T) {
	ts := &util.FakeTimeSource{}

	for _, tc := range []struct {
		desc   string
		errs   stub.Errors
		cancel bool
		want   error
	}{
		{desc: "Start", errs: stub.Errors{Start: stubErr}, want: stubErr},
		{desc: "WaitForMastership", errs: stub.Errors{Wait: stubErr}, want: stubErr},
		{desc: "IsMaster", errs: stub.Errors{IsMaster: stubErr}, want: nil},
		{desc: "Resign", errs: stub.Errors{Resign: stubErr}, want: nil},
		{desc: "cancel", cancel: true, want: context.Canceled},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}

			me := stub.NewMasterElection(true, &tc.errs)
			runner := NewRunner(&cfg, me, ts, "")
			run, err := runner.AwaitMastership(ctx)
			if got, want := err, tc.want; got != want {
				t.Errorf("AwaitMastership(): error=%v, want %v", got, want)
			}
			if got, want := (run != nil), (tc.want == nil); got != want {
				t.Errorf("AwaitMastership() returned run %v, want %v", got, want)
			}
		})
	}
}

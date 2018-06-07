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

func TestRunner_MasterContext(t *testing.T) {
	ctx := context.Background()
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	if runner.MasterContext() != nil {
		t.Errorf("MasterContext(): expected nil")
	}

	testOneRun := func(ctx context.Context) context.Context {
		me.Update(true, nil)
		if err := runner.Start(ctx); err != nil {
			t.Fatalf("Start(): %v", err)
		}
		mctx := runner.MasterContext()
		if mctx == nil {
			t.Errorf("MasterContext(): expected non-nil")
		}
		if err := mctx.Err(); err != nil {
			t.Errorf("MasterContext error: %v, want nil", err)
		}
		runner.Resign(ctx)
		if got, want := mctx.Err(), context.Canceled; got != want {
			t.Errorf("MasterContext error: %v, want %v", got, want)
		}
		if got, want := runner.MasterContext(), mctx; got != want {
			t.Errorf("MasterContext(): %+v, want %+v", got, want)
		}
		return mctx
	}

	ctx1 := testOneRun(ctx)
	ctx2 := testOneRun(ctx)
	if ctx1 == ctx2 {
		t.Errorf("Got same contexts")
	}
}

func TestRunner_Start(t *testing.T) {
	ctx := context.Background()
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")

	isMaster := make(chan struct{})
	var err error
	go func() {
		defer close(isMaster)
		err = runner.Start(ctx)
	}()
	expectNotClosed(t, isMaster, "Became master unexpectedly")

	me.Update(true, nil)
	<-isMaster // Now should become the master.
	if err != nil {
		t.Fatalf("Start(): %v", err)
	}

	mctx := runner.MasterContext()
	expectNotClosed(t, mctx.Done(), "Closed master context unexpectedly")

	if is, _ := me.IsMaster(ctx); !is {
		t.Errorf("Unexpected masteship resignation")
	}
}

func TestRunner_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	me, ts := stub.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	mctx := runner.MasterContext()
	expectNotClosed(t, mctx.Done(), "Closed master context unexpectedly")
	cancel()
	<-mctx.Done() // Runner should terminate.

	if is, _ := me.IsMaster(ctx); !is {
		t.Error("Unexpected masteship resignation")
	}
}

func TestRunner_Resign(t *testing.T) {
	ctx := context.Background()
	me, ts := stub.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	mctx := runner.MasterContext()
	expectNotClosed(t, mctx.Done(), "Closed master context unexpectedly")
	if err := runner.Resign(ctx); err != nil {
		t.Fatalf("Resign(): %v", err)
	}
	<-mctx.Done() // Runner should terminate.

	if is, _ := me.IsMaster(ctx); is {
		t.Error("Expected masteship resignation")
	}
}

func TestRunner_Close(t *testing.T) {
	ctx := context.Background()
	me, ts := stub.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	mctx := runner.MasterContext()
	expectNotClosed(t, mctx.Done(), "Closed master context unexpectedly")
	if err := runner.Close(ctx); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	<-mctx.Done() // Runner should terminate.

	if is, _ := me.IsMaster(ctx); !is {
		t.Error("Unexpected masteship resignation")
	}
}

func TestRunner_ErrorWhileMaster(t *testing.T) {
	ctx := context.Background()
	me, ts := stub.NewMasterElection(true, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	me.Update(true, &stub.Errors{IsMaster: stubErr})
	<-runner.MasterContext().Done() // Mastership monitoring should terminate.

	if is, _ := me.IsMaster(ctx); !is {
		t.Errorf("Unexpected masteship resignation")
	}
}

func TestRunner_HealthyLoop(t *testing.T) {
	me, ts := stub.NewMasterElection(false, nil), &util.FakeTimeSource{}
	runner := NewRunner(&cfg, me, ts, "")
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		me.Update(true, nil)
		if err := runner.Start(ctx); err != nil {
			t.Fatalf("Start(): %v", err)
		}
		// ... do some work as master ...
		if err := runner.Resign(ctx); err != nil {
			t.Fatalf("Resign(): %v", err)
		}
		if is, _ := me.IsMaster(ctx); is {
			t.Errorf("Expected masteship resignation")
		}
	}

	if err := runner.Close(ctx); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

func TestRunner_StartErrors(t *testing.T) {
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
			err := runner.Start(ctx)
			if got, want := err, tc.want; got != want {
				t.Errorf("AwaitMastership(): error=%v, want %v", got, want)
			}
		})
	}
}

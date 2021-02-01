// Copyright 2018 Google LLC. All Rights Reserved.
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

package clock

import (
	"fmt"
	"testing"
	"time"
)

func checkNotFiring(t *testing.T, timer Timer) {
	t.Helper()
	select {
	case tm := <-timer.Chan():
		t.Errorf("Timer unexpectedly fired at %v", tm)
	case <-time.After(10 * time.Millisecond): // Give some real time to pass.
	}
}

func TestFakeTimerFiresOnce(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 0, 0, 0, time.UTC)
	ts := NewFake(base)
	timer := ts.NewTimer(10 * time.Millisecond)

	checkNotFiring(t, timer)
	newTime := base.Add(9 * time.Millisecond)
	ts.Set(newTime)
	checkNotFiring(t, timer)

	newTime = newTime.Add(1 * time.Millisecond)
	ts.Set(newTime)
	got := <-timer.Chan() // Now it should fire.
	if !got.Equal(newTime) {
		t.Errorf("Timer fired at %v, want %v", got, newTime)
	}

	checkNotFiring(t, timer) // Shouldn't fire any more.
}

func TestFakeTimerStopBeforeFire(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 0, 0, 0, time.UTC)
	ts := NewFake(base)
	timer := ts.NewTimer(10 * time.Millisecond)

	checkNotFiring(t, timer)
	if !timer.Stop() {
		t.Error("Stop() returns true, want false")
	}

	newTime := base.Add(20 * time.Millisecond)
	ts.Set(newTime)
	checkNotFiring(t, timer) // Shouldn't fire because it was stopped.
}

func TestFakeTimerStopAfterFire(t *testing.T) {
	for _, drain := range []bool{false, true} {
		t.Run(fmt.Sprintf("drain:%v", drain), func(t *testing.T) {
			base := time.Date(2018, 12, 12, 18, 0, 0, 0, time.UTC)
			ts := NewFake(base)
			timer := ts.NewTimer(10 * time.Millisecond)
			ts.Set(base.Add(20 * time.Millisecond)) // Triggers the event.
			if drain {
				<-timer.Chan()
			}
			if timer.Stop() {
				t.Error("Stop() returns false, want true")
			}
		})
	}
}

func TestManyFakeTimers(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 0, 0, 0, time.UTC)
	ts := NewFake(base)
	var timers []Timer
	var times []time.Time
	for i := 1; i <= 10; i++ {
		d := time.Duration(i) * time.Second
		timers = append(timers, ts.NewTimer(d))
		times = append(times, base.Add(d))
	}
	check := func(fire int, want time.Time) {
		for i, timer := range timers {
			if i != fire {
				checkNotFiring(t, timer)
				continue
			}
			got := <-timer.Chan()
			if !got.Equal(want) {
				t.Errorf("Timer %d fired at %v, want %v", i, got, want)
			}
		}
	}
	check(-1, time.Time{}) // No firing before.
	for i, fTime := range times {
		ts.Set(fTime)
		check(i, fTime)
	}
	check(-1, time.Time{}) // No firing after.
}

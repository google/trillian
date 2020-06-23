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

import "time"

// Timer represents an event that fires with time passage.
// See time.Timer type for intuition on how it works.
type Timer interface {
	// Chan returns a channel which is used to deliver the event.
	Chan() <-chan time.Time
	// Stop prevents the Timer from firing. Returns false if the event has
	// already fired, or the Timer has been stopped.
	Stop() bool
}

// systemTimer is a Timer that uses system time.
type systemTimer struct {
	*time.Timer
}

func (t systemTimer) Chan() <-chan time.Time {
	return t.C
}

// fakeTimer implements Timer interface for testing. Event firing is controlled
// by FakeTimeSource which creates and owns fakeTimer instances.
type fakeTimer struct {
	ts   *FakeTimeSource
	id   int
	when time.Time
	ch   chan time.Time
}

func newFakeTimer(ts *FakeTimeSource, id int, when time.Time) *fakeTimer {
	ch := make(chan time.Time, 1)
	return &fakeTimer{ts: ts, id: id, when: when, ch: ch}
}

func (t *fakeTimer) Chan() <-chan time.Time {
	return t.ch
}

func (t *fakeTimer) Stop() bool {
	return t.ts.unsubscribe(t.id)
}

func (t *fakeTimer) tryFire(now time.Time) bool {
	if t.when.Before(now) || t.when.Equal(now) {
		select {
		case t.ch <- now:
			return true
		default:
		}
	}
	return false
}

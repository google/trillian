// Copyright 2016 Google Inc. All Rights Reserved.
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

package util

import (
	"sync"
	"time"
)

// TimeSource can provide the current time, or be replaced by a mock in tests to return
// specific values.
type TimeSource interface {
	// Now returns the current time in real implementations or a suitable value in others
	Now() time.Time
	// NewTimer creates a timer that fires after the specified duration.
	NewTimer(d time.Duration) Timer
}

// SecondsSince returns the time in seconds elapsed since t until now, as
// measured by the TimeSource.
func SecondsSince(ts TimeSource, t time.Time) float64 {
	return ts.Now().Sub(t).Seconds()
}

// SystemTimeSource provides the current system local time
type SystemTimeSource struct{}

// Now returns the true current local time.
func (s SystemTimeSource) Now() time.Time {
	return time.Now()
}

// NewTimer returns a real timer.
func (s SystemTimeSource) NewTimer(d time.Duration) Timer {
	return systemTimer{time.NewTimer(d)}
}

// FakeTimeSource provides time that can be arbitrarily set. For tests only.
type FakeTimeSource struct {
	mu     sync.RWMutex
	now    time.Time
	timers map[int]*fakeTimer
	nextID int
}

// NewFakeTimeSource creates a FakeTimeSource instance.
func NewFakeTimeSource(t time.Time) *FakeTimeSource {
	timers := make(map[int]*fakeTimer)
	return &FakeTimeSource{now: t, timers: timers}
}

// Now returns the time value this instance contains.
func (f *FakeTimeSource) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.now
}

// NewTimer returns a fake Timer.
func (f *FakeTimeSource) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID
	f.nextID++
	timer := newFakeTimer(f, id, f.now.Add(d))
	f.timers[id] = timer
	return timer
}

// unsubscribe removes the Timer with the specified ID if it exists, and
// returns the existence bit.
func (f *FakeTimeSource) unsubscribe(id int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.timers[id]
	if ok {
		delete(f.timers, id)
	}
	return ok
}

// Set updates the time that this instance will report.
func (f *FakeTimeSource) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
	for id, timer := range f.timers {
		if timer.tryFire(t) {
			delete(f.timers, id)
		}
	}
}

// IncrementingFakeTimeSource takes a base time and several increments, which will be applied to
// the base time each time Now() is called. The first call will return the base time + zeroth
// increment. If called more times than provided for then it will panic. Does not require that
// increments increase monotonically.
type IncrementingFakeTimeSource struct {
	BaseTime      time.Time
	Increments    []time.Duration
	NextIncrement int
}

// Now returns the current time according to this time source, which depends on how many times
// this method has already been invoked.
func (a *IncrementingFakeTimeSource) Now() time.Time {
	adjustedTime := a.BaseTime.Add(a.Increments[a.NextIncrement])
	a.NextIncrement++

	return adjustedTime
}

// NewTimer creates a timer with the specified delay. Not implemented.
func (a *IncrementingFakeTimeSource) NewTimer(d time.Duration) Timer {
	panic("NewTimer: not implemented")
}

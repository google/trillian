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
	"time"
)

// TimeSource can provide the current time, or be replaced by a mock in tests to return
// specific values.
type TimeSource interface {
	// Now returns the current time in real implementations or a suitable value in others
	Now() time.Time
}

// SystemTimeSource provides the current system local time
type SystemTimeSource struct{}

// Now returns the true current local time.
func (s SystemTimeSource) Now() time.Time {
	return time.Now()
}

// FakeTimeSource provides a time that can be any arbitrarily set value for use in tests.
// It should not be used in production code.
type FakeTimeSource struct {
	// FakeTime is the value that this fake time source will return. It is public so
	// tests can manipulate it.
	FakeTime time.Time
}

// Now returns the time value this instance contains
func (f FakeTimeSource) Now() time.Time {
	return f.FakeTime
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

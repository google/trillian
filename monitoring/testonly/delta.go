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

package testonly

import "github.com/google/trillian/monitoring"

// CounterSnapshot records the latest value from a time series in a counter.
// This value can then be compared with future values. Note that a counter can
// contain many time series, but a CounterSnapshot will track only one.
// A CounterSnapshot is useful in tests because counters do not reset between
// test cases, and so their absolute value is not amenable to testing. Instead,
// the delta between the value at the start and end of the test should be used.
// This assumes that multiple tests that all affect the counter are not run in
// parallel.
type CounterSnapshot struct {
	c      monitoring.Counter
	labels []string
	value  float64
}

// NewCounterSnapshot records the latest value of a time series in c identified
// by the given labels. This value can be compared to future values to determine
// how it has changed over time.
func NewCounterSnapshot(c monitoring.Counter, labels ...string) CounterSnapshot {
	return CounterSnapshot{
		c:      c,
		labels: labels,
		value:  c.Value(labels...),
	}
}

// Delta returns the difference between the latest value of the time series
// and the value when the CounterSnapshot was created.
func (s CounterSnapshot) Delta() float64 {
	return s.c.Value(s.labels...) - s.value
}

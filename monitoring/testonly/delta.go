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

package testonly

import (
	"fmt"
	"strings"

	"github.com/google/trillian/monitoring"
)

// CounterSnapshot records the value of a counter at a specific point in time.
type CounterSnapshot struct {
	c      monitoring.Counter
	values map[string]float64
}

// NewCounterSnapshot creates a CounterSnapshot that can record values from a
// counter and later report the delta between those recorded values and the
// current values.
func NewCounterSnapshot(c monitoring.Counter) CounterSnapshot {
	s := CounterSnapshot{
		c:      c,
		values: make(map[string]float64),
	}
	return s
}

// Record stores the current value of the counter.
func (s CounterSnapshot) Record(labels ...string) {
	s.values[keyForLabels(labels...)] = s.c.Value(labels...)
}

// Delta returns the difference between the current value of a counter and its
// value when Record() was last called.
func (s CounterSnapshot) Delta(labels ...string) float64 {
	if oldValue, ok := s.values[keyForLabels(labels...)]; ok {
		return s.c.Value(labels...) - oldValue
	}
	// This is a testonly utility so it is reasonable to panic when misused.
	panic(fmt.Sprintf("No snapshot found for %v", labels))
}

func keyForLabels(labels ...string) string {
	// Assumes that no label contains the '|' character.
	return strings.Join(labels, "|")
}

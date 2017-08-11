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

package quota

import (
	"fmt"
	"sync"

	"github.com/google/trillian/monitoring"
)

var (

	// Metrics groups all quota-related metrics.
	// The metrics represented here are not meant to be maintained by the quota subsystem
	// implementation.  Instead, they're meant to be updated by the quota's callers, in order to
	// record their interactions with quotas.
	// The quota implementation is encouraged to define its own metrics to monitor its internal
	// state.
	Metrics     = &m{}
	metricsOnce = sync.Once{}
)

type m struct {
	AcquiredTokens    monitoring.Counter
	ReturnedTokens    monitoring.Counter
	ReplenishedTokens monitoring.Counter
}

// IncAcquired increments the AcquiredTokens metric.
func (m *m) IncAcquired(tokens int, specs []Spec, success bool) {
	m.add(m.AcquiredTokens, tokens, specs, success)
}

// IncReturned increments the ReturnedTokens metric.
func (m *m) IncReturned(tokens int, specs []Spec, success bool) {
	m.add(m.ReturnedTokens, tokens, specs, success)
}

// IncReplenished increments the ReplenishedTokens metric.
func (m *m) IncReplenished(tokens int, specs []Spec, success bool) {
	m.add(m.ReplenishedTokens, tokens, specs, success)
}

func (m *m) add(c monitoring.Counter, tokens int, specs []Spec, success bool) {
	if c == nil {
		return
	}
	for _, spec := range specs {
		c.Add(float64(tokens), spec.Name(), fmt.Sprint(success))
	}
}

// InitMetrics initializes Metrics using mf to create the monitoring objects.
// May be called multiple times. If so, the first call is the one that counts.
func InitMetrics(mf monitoring.MetricFactory) {
	metricsOnce.Do(func() {
		Metrics.AcquiredTokens = mf.NewCounter("quota_acquired_tokens", "Number of acquired quota tokens", "spec", "success")
		Metrics.ReturnedTokens = mf.NewCounter("quota_returned_tokens", "Number of quota tokens returned due to overcharging (bad requests, duplicates, etc)", "spec", "success")
		Metrics.ReplenishedTokens = mf.NewCounter("quota_replenished_tokens", "Number of quota tokens replenished due to sequencer progress", "spec", "success")
	})
}

// Copyright 2017 Google LLC. All Rights Reserved.
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

package monitoring

import (
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
)

// InertMetricFactory creates inert metrics for testing.
type InertMetricFactory struct{}

// NewCounter creates a new inert Counter.
func (imf InertMetricFactory) NewCounter(name, help string, labelNames ...string) Counter {
	return &InertFloat{
		labelCount: len(labelNames),
		vals:       make(map[string]float64),
	}
}

// NewGauge creates a new inert Gauge.
func (imf InertMetricFactory) NewGauge(name, help string, labelNames ...string) Gauge {
	return &InertFloat{
		labelCount: len(labelNames),
		vals:       make(map[string]float64),
	}
}

// NewHistogram creates a new inert Histogram.
func (imf InertMetricFactory) NewHistogram(name, help string, labelNames ...string) Histogram {
	return &InertDistribution{
		labelCount: len(labelNames),
		counts:     make(map[string]uint64),
		sums:       make(map[string]float64),
	}
}

// NewHistogramWithBuckets creates a new inert Histogram with supplied buckets.
// The buckets are not actually used.
func (imf InertMetricFactory) NewHistogramWithBuckets(name, help string, _ []float64, labelNames ...string) Histogram {
	return imf.NewHistogram(name, help, labelNames...)
}

// InertFloat is an internal-only implementation of both the Counter and Gauge interfaces.
type InertFloat struct {
	labelCount int
	mu         sync.Mutex
	vals       map[string]float64
}

// Inc adds 1 to the value.
func (m *InertFloat) Inc(labelVals ...string) {
	m.Add(1.0, labelVals...)
}

// Dec subtracts 1 from the value.
func (m *InertFloat) Dec(labelVals ...string) {
	m.Add(-1.0, labelVals...)
}

// Add adds the given amount to the value.
func (m *InertFloat) Add(val float64, labelVals ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, err := keyForLabels(labelVals, m.labelCount)
	if err != nil {
		glog.Error(err.Error())
		return
	}
	m.vals[key] += val
}

// Set sets the value.
func (m *InertFloat) Set(val float64, labelVals ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, err := keyForLabels(labelVals, m.labelCount)
	if err != nil {
		glog.Error(err.Error())
		return
	}
	m.vals[key] = val
}

// Value returns the current value.
func (m *InertFloat) Value(labelVals ...string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, err := keyForLabels(labelVals, m.labelCount)
	if err != nil {
		glog.Error(err.Error())
		return 0.0
	}
	return m.vals[key]
}

// InertDistribution is an internal-only implementation of the Distribution interface.
type InertDistribution struct {
	labelCount int
	mu         sync.Mutex
	counts     map[string]uint64
	sums       map[string]float64
}

// Observe adds a single observation to the distribution.
func (m *InertDistribution) Observe(val float64, labelVals ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, err := keyForLabels(labelVals, m.labelCount)
	if err != nil {
		glog.Error(err.Error())
		return
	}
	m.counts[key]++
	m.sums[key] += val
}

// Info returns count, sum for the distribution.
func (m *InertDistribution) Info(labelVals ...string) (uint64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, err := keyForLabels(labelVals, m.labelCount)
	if err != nil {
		glog.Error(err.Error())
		return 0, 0.0
	}
	return m.counts[key], m.sums[key]
}

func keyForLabels(labelVals []string, count int) (string, error) {
	if len(labelVals) != count {
		return "", fmt.Errorf("invalid label count %d; want %d", len(labelVals), count)
	}
	return strings.Join(labelVals, "|"), nil
}

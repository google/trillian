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

// MetricFactory allows the creation of different types of metric.
type MetricFactory interface {
	NewCounter(name, help string, labelNames ...string) Counter
	NewGauge(name, help string, labelNames ...string) Gauge
	NewHistogram(name, help string, labelNames ...string) Histogram
	NewHistogramWithBuckets(name, help string, buckets []float64, labelNames ...string) Histogram
}

// Counter is a metric class for numeric values that increase.
type Counter interface {
	Inc(labelVals ...string)
	Add(val float64, labelVals ...string)
	Value(labelVals ...string) float64
}

// Gauge is a metric class for numeric values that can go up and down.
type Gauge interface {
	Inc(labelVals ...string)
	Dec(labelVals ...string)
	Add(val float64, labelVals ...string)
	Set(val float64, labelVals ...string)
	// Value retrieves the value for a particular set of labels.
	// This is only really useful for testing implementations.
	Value(labelVals ...string) float64
}

// Histogram is a metric class that tracks the distribution of a collection
// of observations.
type Histogram interface {
	Observe(val float64, labelVals ...string)
	// Info retrieves the count and sum of observations for a particular set of labels.
	// This is only really useful for testing implementations.
	Info(labelVals ...string) (uint64, float64)
}

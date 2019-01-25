// Copyright 2019 Google Inc. All Rights Reserved.
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
	"math"
)

// This file contains helpers for constructing buckets for use with
// Histogram metrics.

// PercentileBuckets returns a range of buckets for 0.0-100.0% use cases.
// in specified integer increments. The increment must be at least 1%, which
// prevents creating very large metric exports.
func PercentileBuckets(inc int64) []float64 {
	if inc <= 0 || inc > 100 {
		return nil
	}
	r := make([]float64, 0, 100/inc)
	var v int64
	for v < 100 {
		r = append(r, float64(v))
		v += inc
	}
	return r
}

// LatencyBuckets returns a reasonable range of histogram upper limits for most
// latency-in-seconds usecases.
func LatencyBuckets() []float64 {
	// These parameters give an exponential range from 0.04 seconds to ~1 day.
	num := 300
	b := 1.05
	scale := 0.04

	r := make([]float64, 0, num)
	for i := 0; i < num; i++ {
		r = append(r, math.Pow(b, float64(i))*scale)
	}
	return r
}

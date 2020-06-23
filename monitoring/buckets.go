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
// latency-in-seconds usecases. The thresholds increase exponentially from 0.04
// seconds to ~282 days.
func LatencyBuckets() []float64 {
	return ExpBuckets(0.04, 1.07, 300)
}

// ExpBuckets returns the specified number of histogram buckets with
// exponentially increasing thresholds. The thresholds vary between base and
// base * mult^(buckets-1).
func ExpBuckets(base, mult float64, buckets uint) []float64 {
	r := make([]float64, buckets)
	for i, exp := uint(0), base; i < buckets; i, exp = i+1, exp*mult {
		r[i] = exp
	}
	return r
}

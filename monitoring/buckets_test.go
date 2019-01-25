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
	"fmt"
	"math"
	"testing"
)

func TestPercentileBucketsInvalid(t *testing.T) {
	for _, inc := range []int64{0, -1, -50, 300, 40000000} {
		t.Run(fmt.Sprintf("increment %d", inc), func(t *testing.T) {
			if got := PercentileBuckets(inc); got != nil {
				t.Errorf("PercentileBuckets: got: %v for invalid case, want: nil", got)
			}
		})
	}
}

func TestPercentileBuckets(t *testing.T) {
	for _, inc := range []int64{1, 2, 10, 25, 46, 97} {
		t.Run(fmt.Sprintf("increment %d", inc), func(t *testing.T) {
			buckets := PercentileBuckets(inc)
			// The number of buckets expected to be created is fixed.
			if got, want := len(buckets), int(100/inc); math.Abs(float64(got-want)) > 1 {
				t.Errorf("PercentileBuckets(): got len: %d, want: %d", got, want)
			}
			// The first bucket should always be close to 0%.
			if buckets[0] < 0 || buckets[0] > 0.0001 {
				t.Errorf("PercentileBuckets(): got first bucket: %v, want: ~0.0", buckets[0])
			}
			// The last bucket should be on the way towards 100%. It doesn't make a
			// lot of sense to create an extremely coarse grained distribution but
			// it's not actually wrong so no reason to reject it.
			if got, want := math.Abs(buckets[len(buckets)-1]-75.0), 25.0; got > want {
				t.Errorf("PercentileBuckets(): got last bucket diff: %v, want: <%v", got, want)
			}
			// Percentile buckets should increase monotonically.
			for i := 0; i < len(buckets)-1; i++ {
				if buckets[i] > buckets[i+1] {
					t.Errorf("PercentileBuckets(): buckets out of order at index: %d", i)
				}
			}
		})
	}
}

func TestLatencyBuckets(t *testing.T) {
	// Just do some probes on the result to make sure it looks sensible.
	buckets := LatencyBuckets()
	// Lowest bucket should be about 0.04 sec.
	if math.Abs(buckets[0]-0.04) > 0.001 {
		t.Errorf("PercentileBuckets(): got first bucket: %v, want: ~0.04", buckets[0])
	}
	// Highest bucket should be about 86400 sec = 1 day (allow some leeway)/
	if got, want := math.Abs(buckets[len(buckets)-1]-86400.0), 300.0; got > want {
		t.Errorf("PercentileBuckets(): got last bucket diff: %v, want: <%v", got, want)
	}
	// Latency buckets should increase monotonically.
	for i := 0; i < len(buckets)-1; i++ {
		if buckets[i] > buckets[i+1] {
			t.Errorf("LatencyBuckets(): buckets out of order at index: %d", i)
		}
	}
}

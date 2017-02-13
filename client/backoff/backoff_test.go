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

package backoff

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	b := Backoff{
		Min:    time.Duration(1),
		Max:    time.Duration(100),
		Factor: 2,
	}
	for _, test := range []struct {
		b     Backoff
		times int
		want  time.Duration
	}{
		{b, 1, time.Duration(1)},
		{b, 2, time.Duration(2)},
		{b, 3, time.Duration(4)},
		{b, 4, time.Duration(8)},
		{b, 8, time.Duration(100)},
	} {
		test.b.Reset()
		var got time.Duration
		for i := 0; i < test.times; i++ {
			got = test.b.Duration()
		}
		if got != test.want {
			t.Errorf("Duration() %v times: %v, want %v", test.times, got, test.want)
		}
	}
}

func TestJitter(t *testing.T) {
	b := Backoff{
		Min:    time.Duration(1) * time.Second,
		Max:    time.Duration(100) * time.Second,
		Factor: 2,
		Jitter: true,
	}
	for _, test := range []struct {
		b     Backoff
		times int
		min   time.Duration
		max   time.Duration
	}{
		{b, 1, time.Duration(1) * time.Second, time.Duration(2) * time.Second},
		{b, 2, time.Duration(2) * time.Second, time.Duration(4) * time.Second},
		{b, 3, time.Duration(4) * time.Second, time.Duration(8) * time.Second},
		{b, 4, time.Duration(8) * time.Second, time.Duration(16) * time.Second},
		{b, 8, time.Duration(100) * time.Second, time.Duration(200) * time.Second},
	} {
		test.b.Reset()
		var got1 time.Duration
		for i := 0; i < test.times; i++ {
			got1 = test.b.Duration()
		}
		if got1 < test.min || got1 > test.max {
			t.Errorf("Duration() %v times, want  %v < %v < %v", test.times, test.min, got1, test.max)
		}

		// Ensure a random value is being produced.
		test.b.Reset()
		var got2 time.Duration
		for i := 0; i < test.times; i++ {
			got2 = test.b.Duration()
		}
		if got1 == got2 {
			t.Errorf("Duration() %v times == Duration() %v times, want  %v != %v",
				test.times, test.times, got1, got2)
		}
	}
}

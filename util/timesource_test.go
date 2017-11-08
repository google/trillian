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
	"testing"
	"time"
)

func TestFakeTimeSource(t *testing.T) {
	date1 := time.Date(1970, 9, 19, 12, 00, 00, 00, time.UTC)
	date2 := time.Date(2007, 7, 7, 11, 35, 00, 00, time.UTC)
	fake := NewFakeTimeSource(date1)

	// Check that a FakeTimeSource can be used as a TimeSource.
	var ts TimeSource = fake
	if got, want := ts.Now(), date1; got != want {
		t.Errorf("ts.Now=%v; want %v", got, want)
	}

	fake.Set(date2)
	if got, want := ts.Now(), date2; got != want {
		t.Errorf("ts.Now=%v; want %v", got, want)
	}
}

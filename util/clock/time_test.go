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

package clock

import (
	"context"
	"fmt"
	"testing"
	"time"
)

var (
	cases = []struct {
		dur     time.Duration
		timeout time.Duration
		cancel  bool
		wantErr error
	}{
		{dur: 0 * time.Second, timeout: time.Second},
		{dur: 10 * time.Millisecond, timeout: 20 * time.Millisecond},
		{dur: 20 * time.Millisecond, timeout: 10 * time.Millisecond, wantErr: context.DeadlineExceeded},
		{dur: 1 * time.Millisecond, timeout: 0 * time.Second, wantErr: context.DeadlineExceeded},
		{dur: 10 * time.Millisecond, timeout: 20 * time.Millisecond, cancel: true, wantErr: context.Canceled},
	}
)

func TestSleep(t *testing.T) {
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%v:%v", tc.dur, tc.timeout), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			if tc.cancel {
				cancel()
			} else {
				defer cancel()
			}
			if got, want := Sleep(ctx.Done(), tc.dur), (tc.wantErr == nil); got != want {
				t.Errorf("Sleep() returned %v, want %v", got, want)
			}
		})
	}
}

func TestSleepContext(t *testing.T) {
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%v:%v", tc.dur, tc.timeout), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			if tc.cancel {
				cancel()
			} else {
				defer cancel()
			}
			if got, want := SleepContext(ctx, tc.dur), tc.wantErr; got != want {
				t.Errorf("SleepContext() returned %v, want %v", got, want)
			}
		})
	}
}

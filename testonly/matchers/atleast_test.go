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

package matchers

import (
	"testing"

	_ "k8s.io/klog/v2"
)

func TestAtLeast(t *testing.T) {
	tests := []struct {
		num, actual int
		want        bool
	}{
		{num: 5, actual: 3},
		{num: 5, actual: 5, want: true},
		{num: 5, actual: 10, want: true},
	}
	for _, test := range tests {
		if got := AtLeast(test.num).Matches(test.actual); got != test.want {
			t.Errorf("AtLeast(%v).Matches(%v) = %v, want = %v", test.num, test.actual, got, test.want)
		}
	}
}

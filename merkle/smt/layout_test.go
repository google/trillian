// Copyright 2021 Google LLC. All Rights Reserved.
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

package smt

import "testing"

func TestLayoutLocate(t *testing.T) {
	l1 := NewLayout([]int{1, 2, 4, 8})
	l2 := NewLayout([]int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176})
	for _, tc := range []struct {
		desc  string
		l     Layout
		depth uint
		wantD uint
		wantH uint
	}{
		{desc: "l1-root", l: l1, depth: 0, wantD: 0, wantH: 0},
		{desc: "l1-1", l: l1, depth: 1, wantD: 0, wantH: 1},
		{desc: "l1-2", l: l1, depth: 2, wantD: 1, wantH: 2},
		{desc: "l1-3", l: l1, depth: 3, wantD: 1, wantH: 2},
		{desc: "l1-4", l: l1, depth: 4, wantD: 3, wantH: 4},
		{desc: "l1-5", l: l1, depth: 5, wantD: 3, wantH: 4},
		{desc: "l1-7", l: l1, depth: 7, wantD: 3, wantH: 4},
		{desc: "l1-8", l: l1, depth: 8, wantD: 7, wantH: 8},
		{desc: "l1-10", l: l1, depth: 10, wantD: 7, wantH: 8},
		{desc: "l1-leaves", l: l1, depth: 15, wantD: 7, wantH: 8},

		{desc: "l2-root", l: l2, depth: 0, wantD: 0, wantH: 0},
		{desc: "l2-3", l: l2, depth: 3, wantD: 0, wantH: 8},
		{desc: "l2-8", l: l2, depth: 8, wantD: 0, wantH: 8},
		{desc: "l2-10", l: l2, depth: 10, wantD: 8, wantH: 8},
		{desc: "l2-20", l: l2, depth: 20, wantD: 16, wantH: 8},
		{desc: "l2-63", l: l2, depth: 63, wantD: 56, wantH: 8},
		{desc: "l2-64", l: l2, depth: 64, wantD: 56, wantH: 8},
		{desc: "l2-79", l: l2, depth: 79, wantD: 72, wantH: 8},
		{desc: "l2-80", l: l2, depth: 80, wantD: 72, wantH: 8},
		{desc: "l2-81", l: l2, depth: 81, wantD: 80, wantH: 176},
		{desc: "l2-100", l: l2, depth: 100, wantD: 80, wantH: 176},
		{desc: "l2-leaves", l: l2, depth: 256, wantD: 80, wantH: 176},
		{desc: "l2-out-of-range", l: l2, depth: 257, wantD: 256, wantH: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			d, h := tc.l.Locate(tc.depth)
			if got, want := d, tc.wantD; got != want {
				t.Errorf("tier depth is %d, want %d", got, want)
			}
			if got, want := h, tc.wantH; got != want {
				t.Errorf("tier height is %d, want %d", got, want)
			}
		})
	}
}

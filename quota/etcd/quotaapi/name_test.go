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

package quotaapi

import "testing"

func TestNewNameFilter(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "quotas/global/read/config"},
		{name: "quotas/global/write/config"},
		{name: "quotas/global/-/config"},           // all global
		{name: "quotas/-/-/config", wantErr: true}, // not allowed, use quotas/global instead

		{name: "quotas/trees/nan/read/config", wantErr: true}, // ID must be a number
		{name: "quotas/trees/1/read/config"},
		{name: "quotas/trees/12345/read/config"},
		{name: "quotas/trees/12345/write/config"},
		{name: "quotas/trees/-/read/config"},  // all trees/read
		{name: "quotas/trees/-/write/config"}, // all trees/write
		{name: "quotas/trees/12345/-/config"}, // all quotas for tree 12345
		{name: "quotas/trees/-/-/config"},     // all trees

		{name: "quotas/users/u/read/config"},
		{name: "quotas/users/llama/read/config"},
		{name: "quotas/users/llama/write/config"},
		{name: "quotas/users/-/read/config"},  // all users/read
		{name: "quotas/users/-/write/config"}, // all users/write
		{name: "quotas/users/llama/-/config"}, // all quotas for user "llama"
		{name: "quotas/users/-/-/config"},     // all users

		{name: "quotas/-/1/read/config", wantErr: true}, // not allowed, use either trees/1 or users/1
		{name: "quotas/-/1/write/config", wantErr: true},
		{name: "quotas/-/1/-/config", wantErr: true},
		{name: "quotas/-/x/read/config", wantErr: true},
		{name: "quotas/-/-/read/config"},  // all trees/read and users/read
		{name: "quotas/-/-/write/config"}, // all trees/write and users/write
		{name: "quotas/-/-/-/config"},     // all trees and users

		{name: "bad/global/read/config", wantErr: true},
		{name: "quotas/global/read/bad", wantErr: true},
		{name: "bad/trees/1/read/config", wantErr: true},
		{name: "quotas/trees/1/read/bad", wantErr: true},
		{name: "bogus/quotas/trees/1/read/config", wantErr: true},
		{name: "quotas/trees/1/read/config/trailingbogus", wantErr: true},
	}
	for _, test := range tests {
		_, err := newNameFilter(test.name)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("newNameFilter(%q) returned err = %v, wantErr = %v", test.name, err, test.wantErr)
		}
	}
}

func TestNameFilter_Matches(t *testing.T) {
	tests := []struct {
		nf                     string
		wantMatch, wantNoMatch []string
	}{
		{
			nf:          "quotas/global/read/config",
			wantMatch:   []string{"quotas/global/read/config"},
			wantNoMatch: []string{"quotas/global/write/config"},
		},
		{
			nf:        "quotas/global/write/config",
			wantMatch: []string{"quotas/global/write/config"},
		},
		{
			nf:          "quotas/trees/1/read/config",
			wantMatch:   []string{"quotas/trees/1/read/config"},
			wantNoMatch: []string{"quotas/trees/2/read/config"},
		},
		{
			nf:        "quotas/trees/1/write/config",
			wantMatch: []string{"quotas/trees/1/write/config"},
		},
		{
			nf:          "quotas/users/llama/read/config",
			wantMatch:   []string{"quotas/users/llama/read/config"},
			wantNoMatch: []string{"quotas/users/alpaca/read/config"},
		},
		{
			nf:        "quotas/users/llama/write/config",
			wantMatch: []string{"quotas/users/llama/write/config"},
		},
		{
			nf: "quotas/global/-/config",
			wantMatch: []string{
				"quotas/global/read/config",
				"quotas/global/write/config",
			},
		},
		{
			nf: "quotas/trees/12345/-/config",
			wantMatch: []string{
				"quotas/trees/12345/read/config",
				"quotas/trees/12345/write/config",
			},
			wantNoMatch: []string{
				"quotas/global/read/config",
				"quotas/trees/1/read/config",
				"quotas/trees/1/write/config",
				"quotas/users/12345/read/config",
			},
		},
		{
			nf: "quotas/trees/-/-/config",
			wantMatch: []string{
				"quotas/trees/1/read/config",
				"quotas/trees/1/write/config",
				"quotas/trees/12345/read/config",
				"quotas/trees/12345/write/config",
			},
			wantNoMatch: []string{
				"quotas/global/read/config",
				"quotas/users/12345/read/config",
			},
		},
		{
			nf: "quotas/users/-/-/config",
			wantMatch: []string{
				"quotas/users/llama/read/config",
				"quotas/users/llama/write/config",
			},
			wantNoMatch: []string{"quotas/trees/12345/read/config"},
		},
		{
			nf: "quotas/-/-/-/config",
			wantMatch: []string{
				"quotas/trees/12345/read/config",
				"quotas/trees/12345/write/config",
				"quotas/users/llama/read/config",
				"quotas/users/llama/write/config",
			},
			wantNoMatch: []string{"quotas/global/read/config"},
		},
	}
	for _, test := range tests {
		nf, err := newNameFilter(test.nf)
		if err != nil {
			t.Errorf("newNameFilter(%q) returned err = %v", test.nf, err)
			continue
		}
		run := func(names []string, want bool) {
			for _, name := range names {
				if got := nf.matches(name); got != want {
					t.Errorf("newNameFilter(%q).matches(%q) = %v, want = %v", test.nf, name, got, want)
				}
			}
		}
		run(test.wantMatch, true)
		run(test.wantNoMatch, false)
	}
}

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

package quota

import (
	"context"
	"testing"
)

func TestNoop_ValidatesNumTokens(t *testing.T) {
	tests := []struct {
		desc      string
		numTokens int
		wantErr   bool
	}{
		{desc: "valid1", numTokens: 1},
		{desc: "valid2", numTokens: 42},
		{desc: "zeroTokens", numTokens: 0, wantErr: true},
		{desc: "negativeTokens", numTokens: -3, wantErr: true},
	}

	ctx := context.Background()
	qm := Noop()
	for _, test := range tests {
		err := qm.GetTokens(ctx, test.numTokens, []Spec{{Group: Global, Kind: Read}})
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}

		err = qm.PutTokens(ctx, test.numTokens, []Spec{{Group: Global, Kind: Read}})
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: PutTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}
	}
}

func TestNoop_ValidatesSpecs(t *testing.T) {
	ctx := context.Background()
	qm := Noop()

	tests := []struct {
		desc    string
		specs   []Spec
		wantErr bool
	}{
		{
			desc:  "validUser",
			specs: []Spec{{Group: User, Kind: Read, User: "dougal"}},
		},
		{
			desc:  "validTree",
			specs: []Spec{{Group: Tree, Kind: Read, TreeID: 12345}},
		},
		{
			desc:  "validGlobal",
			specs: []Spec{{Group: Global, Kind: Read}},
		},
		{
			desc: "validMixed",
			specs: []Spec{
				{Group: User, Kind: Read, User: "brian"},
				{Group: Tree, Kind: Read, TreeID: 12345},
				{Group: Global, Kind: Read},
			},
		},
		{
			desc:    "noTreeID",
			specs:   []Spec{{Group: Tree, Kind: Read}},
			wantErr: true,
		},
		{
			desc:    "negativeTreeID",
			specs:   []Spec{{Group: Tree, Kind: Read, TreeID: -1}},
			wantErr: true,
		},
	}
	for _, test := range tests {
		err := qm.GetTokens(ctx, 1 /* numTokens */, test.specs)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}

		_, err = qm.PeekTokens(ctx, test.specs)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: PeekTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}

		err = qm.PutTokens(ctx, 1 /* numTokens */, test.specs)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: PutTokens() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}

		err = qm.ResetQuota(ctx, test.specs)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: ResetQuota() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
		}
	}
}

func TestNoopManager_PeekTokens(t *testing.T) {
	ctx := context.Background()
	qm := Noop()

	specs := []Spec{
		{Group: User, Kind: Read, User: "ermintrude"},
		{Group: Tree, Kind: Read, TreeID: 12345},
		{Group: Global, Kind: Read},
	}
	tokens, err := qm.PeekTokens(ctx, specs)
	if err != nil {
		t.Fatalf("PeekTokens() returned err = %v", err)
	}
	if got, want := len(tokens), len(specs); got != want {
		t.Fatalf("len(tokens) = %v, want = %v", got, want)
	}
	want := MaxTokens
	for _, spec := range specs {
		if got := tokens[spec]; got != want {
			t.Errorf("tokens[%#v] = %v, want = %v", spec, got, want)
		}
	}
}

// Copyright 2018 Google LLC. All Rights Reserved.
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

package main

import (
	"context"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/flagsaver"
)

type testCase struct {
	desc       string
	setFlags   func()
	updateErr  error
	wantRPC    bool
	updateTree *trillian.Tree
	wantErr    bool
	wantState  trillian.TreeState
}

func TestFreezeTree(t *testing.T) {
	runTest(t, []*testCase{
		{
			// We don't set the treeID in runTest so this should fail.
			desc:    "missingTreeID",
			wantErr: true,
		},
		{
			desc: "mandatoryOptsNotSet",
			// Undo the flags set by runTest, so that mandatory options are no longer set.
			setFlags: resetFlags,
			wantErr:  true,
		},
		{
			desc: "validUpdateFrozen",
			setFlags: func() {
				*treeID = 12345
				*treeState = "FROZEN"
			},
			wantRPC: true,
			updateTree: &trillian.Tree{
				TreeId:    12345,
				TreeState: trillian.TreeState_FROZEN,
			},
			wantState: trillian.TreeState_FROZEN,
		},
		{
			desc: "updateInvalidState",
			setFlags: func() {
				*treeID = 12345
				*treeState = "ITSCOLDOUTSIDE"
			},
			wantErr: true,
		},
		{
			desc: "unknownTree",
			setFlags: func() {
				*treeID = 123456
				*treeState = "FROZEN"
			},
			wantErr:   true,
			wantRPC:   true,
			updateErr: errors.New("unknown tree id"),
		},
		{
			desc: "emptyAddr",
			setFlags: func() {
				*adminServerAddr = ""
				*treeID = 12345
				*treeState = "FROZEN"
			},
			wantErr: true,
		},
		{
			desc: "updateErr",
			setFlags: func() {
				*treeID = 12345
				*treeState = "FROZEN"
			},
			wantRPC:   true,
			updateErr: errors.New("update tree failed"),
			wantErr:   true,
		},
	})
}

// runTest executes the updateTree command against a fake TrillianAdminServer
// for each of the provided tests, and checks that the tree in the request is
// as expected, or an expected error occurs.
// Prior to each test case, it:
// 1. Resets all flags to their original values.
// 2. Sets the adminServerAddr flag to point to the fake server.
// 3. Calls the test's setFlags func (if provided) to allow it to change flags specific to the test.
func runTest(t *testing.T, tests []*testCase) {
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			// Note: Restore() must be called after the flag-reading bits are
			// stopped, otherwise there might be a data race.
			defer flagsaver.Save().MustRestore()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			s, stopFakeServer, err := testonly.NewMockServer(ctrl)
			if err != nil {
				t.Fatalf("Error starting fake server: %v", err)
			}
			defer stopFakeServer()
			*adminServerAddr = s.Addr
			if tc.setFlags != nil {
				tc.setFlags()
			}

			// We might not get as far as updating the tree on the admin server.
			if tc.wantRPC {
				call := s.Admin.EXPECT().UpdateTree(gomock.Any(), gomock.Any()).Return(tc.updateTree, tc.updateErr)
				expectCalls(call, tc.updateErr)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			tree, err := updateTree(ctx)
			if hasErr := err != nil; hasErr != tc.wantErr {
				t.Errorf("updateTree() returned err = '%v', wantErr = %v", err, tc.wantErr)
				return
			}

			if err == nil {
				if got, want := tree.TreeState.String(), tc.wantState.String(); got != want {
					t.Errorf("updated state incorrect got: %v want: %v", got, want)
				}
			}
		})
	}
}

// expectCalls returns the minimum number of times a function is expected to be called
// given the return error for the function (err), and all previous errors in the function's
// code path.
func expectCalls(call *gomock.Call, err error, prevErr ...error) *gomock.Call {
	// If a function prior to this function errored,
	// we do not expect this function to be called.
	for _, e := range prevErr {
		if e != nil {
			return call.Times(0)
		}
	}
	// If this function errors, it might be retried multiple times.
	if err != nil {
		return call.MinTimes(1)
	}
	// If this function succeeds it should only be called once.
	return call.Times(1)
}

// resetFlags sets all flags to their default values.
func resetFlags() {
	flag.Visit(func(f *flag.Flag) {
		f.Value.Set(f.DefValue)
	})
}

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

package main

import (
	"context"
	"errors"
	"flag"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/cmd/testonly"
	"github.com/google/trillian/util/flagsaver"
)

type testCase struct {
	desc      string
	setFlags  func()
	updateErr error
	wantErr   bool
	wantState trillian.TreeState
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
			desc:     "unknownTree",
			setFlags: func() { *treeID = 123456 },
			wantErr:  true,
		},
		{
			desc:     "emptyAddr",
			setFlags: func() { *adminServerAddr = "" },
			wantErr:  true,
		},
		{
			desc:      "updateErr",
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
	server := &testonly.FakeAdminMapServer{TreeID: 12345}

	lis, stopFakeServer, err := testonly.StartFakeServer(server)
	if err != nil {
		t.Fatalf("Error starting fake server: %v", err)
	}
	defer stopFakeServer()

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			defer flagsaver.Save().Restore()
			*adminServerAddr = lis.Addr().String()
			if test.setFlags != nil {
				test.setFlags()
			}

			server.Err = test.updateErr

			tree, err := updateTree(ctx)
			switch hasErr := err != nil; {
			case hasErr != test.wantErr:
				t.Errorf("updateTree() returned err = '%v', wantErr = %v", err, test.wantErr)
				return
			case hasErr:
				return
			}

			if got, want := tree.TreeState.String(), test.wantState.String(); got != want {
				t.Errorf("updated state incorrect got:%v want:%v", got, want)
			}
		})
	}
}

// resetFlags sets all flags to their default values.
func resetFlags() {
	flag.Visit(func(f *flag.Flag) {
		f.Value.Set(f.DefValue)
	})
}

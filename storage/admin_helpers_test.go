// Copyright 2019 Google LLC. All Rights Reserved.
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

package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"google.golang.org/protobuf/proto"
)

// TestGetTree is really testing param / result / error propagation is correct
// as it wraps RunInAdminSnapshot which has its own tests. The other helpers
// use admin.RunInTransaction, which is provided by the implementation, so
// cannot usefully be tested here.
func TestGetTree(t *testing.T) {
	dummyTree := &trillian.Tree{TreeId: 999, TreeType: trillian.TreeType_LOG}

	tests := []struct {
		name       string
		setup      func(*MockAdminStorage, *MockReadOnlyAdminTX)
		testFn     func(AdminStorage) (*trillian.Tree, error)
		snapErr    error
		wantErrStr string
		wantTree   *trillian.Tree
	}{
		{
			name:       "get snap fail",
			snapErr:    errors.New("SNAP"),
			wantErrStr: "SNAP",
			setup: func(_ *MockAdminStorage, atx *MockReadOnlyAdminTX) {
			},
			testFn: func(as AdminStorage) (*trillian.Tree, error) {
				return GetTree(context.Background(), as, 999)
			},
		},
		{
			name:       "get tree fail",
			wantErrStr: "TREE",
			setup: func(_ *MockAdminStorage, atx *MockReadOnlyAdminTX) {
				atx.EXPECT().GetTree(gomock.Any(), int64(999)).Return(nil, errors.New("TREE"))
			},
			testFn: func(as AdminStorage) (*trillian.Tree, error) {
				return GetTree(context.Background(), as, 999)
			},
		},
		{
			name: "get tree ok",
			setup: func(_ *MockAdminStorage, atx *MockReadOnlyAdminTX) {
				atx.EXPECT().GetTree(gomock.Any(), int64(999)).Return(dummyTree, nil)
				atx.EXPECT().Commit().Return(nil)
			},
			testFn: func(as AdminStorage) (*trillian.Tree, error) {
				return GetTree(context.Background(), as, 999)
			},
			wantTree: dummyTree,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup == nil || tc.testFn == nil {
				t.Fatalf("Bad test case: %v", tc)
			}
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			as := NewMockAdminStorage(ctrl)
			atx := NewMockReadOnlyAdminTX(ctrl)
			as.EXPECT().Snapshot(gomock.Any()).Return(atx, tc.snapErr)
			if tc.snapErr == nil {
				atx.EXPECT().Close().Return(nil)
			}

			tc.setup(as, atx)
			tree, err := tc.testFn(as)
			if len(tc.wantErrStr) == 0 {
				// Expect no error.
				if err != nil {
					t.Errorf("GetTree()=%v,%v, want: err=nil", tree, err)
				}
				if !proto.Equal(tc.wantTree, tree) {
					t.Errorf("GetTree()=%v,%v, want: %v,nil", tree, err, tc.wantTree)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrStr) {
					t.Errorf("GetTree()=%v,%v, want: err with: %s", tree, err, tc.wantErrStr)
				}
			}
		})
	}
}

func TestRunInAdminSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		snapErr    error
		wantCommit bool
		commitErr  error
		wantErrStr string
		fn         func(tx ReadOnlyAdminTX) error
	}{
		{
			name:       "snap fail",
			snapErr:    errors.New("SNAP"),
			wantErrStr: "SNAP",
		},
		{
			name: "tx err",
			fn: func(tx ReadOnlyAdminTX) error {
				return errors.New("TX")
			},
			wantErrStr: "TX",
		},
		{
			name: "commit err",
			fn: func(tx ReadOnlyAdminTX) error {
				return nil
			},
			wantCommit: true,
			commitErr:  errors.New("commit"),
			wantErrStr: "commit",
		},
		{
			name: "tx ok",
			fn: func(tx ReadOnlyAdminTX) error {
				return nil
			},
			wantCommit: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			as := NewMockAdminStorage(ctrl)
			atx := NewMockAdminTX(ctrl)
			as.EXPECT().Snapshot(gomock.Any()).Return(atx, tc.snapErr)
			if tc.snapErr == nil {
				atx.EXPECT().Close().Return(nil)
			}
			if tc.wantCommit {
				atx.EXPECT().Commit().Return(tc.commitErr)
			}

			err := RunInAdminSnapshot(context.Background(), as, tc.fn)
			if len(tc.wantErrStr) == 0 {
				// Expect no error.
				if err != nil {
					t.Errorf("RunInAdminSnapshot()=%v, want: nil", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrStr) {
					t.Errorf("RunInAdminSnapshot()=%v, want: err with: %s", err, tc.wantErrStr)
				}
			}
		})
	}
}

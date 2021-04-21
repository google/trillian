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

package trees

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFromContext(t *testing.T) {
	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "noTree"},
		{desc: "hasTree", tree: testonly.LogTree},
	}
	for _, test := range tests {
		ctx := NewContext(context.Background(), test.tree)

		tree, ok := FromContext(ctx)
		switch wantOK := test.tree != nil; {
		case ok != wantOK:
			t.Errorf("%v: FromContext(%v) = (_, %v), want = (_, %v)", test.desc, ctx, ok, wantOK)
		case ok && !proto.Equal(tree, test.tree):
			t.Errorf("%v: FromContext(%v) = (%v, nil), want = (%v, nil)", test.desc, ctx, tree, test.tree)
		case !ok && tree != nil:
			t.Errorf("%v: FromContext(%v) = (%v, %v), want = (nil, %v)", test.desc, ctx, tree, ok, wantOK)
		}
	}
}

func TestGetTree(t *testing.T) {
	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 1

	frozenTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	frozenTree.TreeId = 3
	frozenTree.TreeState = trillian.TreeState_FROZEN

	drainingTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	drainingTree.TreeId = 3
	drainingTree.TreeState = trillian.TreeState_DRAINING

	softDeletedTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	softDeletedTree.Deleted = true
	softDeletedTree.DeleteTime = timestamppb.Now()

	tests := []struct {
		desc                           string
		treeID                         int64
		opts                           GetOpts
		ctxTree, storageTree, wantTree *trillian.Tree
		beginErr, getErr, commitErr    error
		wantErr                        bool
		code                           codes.Code
	}{
		{
			desc:        "anyTree",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query),
			storageTree: logTree,
			wantTree:    logTree,
		},
		{
			desc:        "logTree",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			storageTree: logTree,
			wantTree:    logTree,
		},
		{
			desc:        "logTreeButMaybePreordered",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG),
			storageTree: logTree,
			wantTree:    logTree,
		},
		{
			desc:        "wrongType1",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_PREORDERED_LOG),
			storageTree: logTree,
			wantErr:     true,
			code:        codes.InvalidArgument,
		},
		{
			desc:        "adminLog",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Admin, trillian.TreeType_LOG),
			storageTree: logTree,
			wantTree:    logTree,
		},
		{
			desc:        "adminPreordered",
			treeID:      testonly.PreorderedLogTree.TreeId,
			opts:        NewGetOpts(Admin, trillian.TreeType_PREORDERED_LOG),
			storageTree: testonly.PreorderedLogTree,
			wantTree:    testonly.PreorderedLogTree,
		},
		{
			desc:        "adminFrozen",
			treeID:      frozenTree.TreeId,
			opts:        NewGetOpts(Admin, trillian.TreeType_LOG),
			storageTree: frozenTree,
			wantTree:    frozenTree,
		},
		{
			desc:        "queryLog",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			storageTree: logTree,
			wantTree:    logTree,
		},
		{
			desc:        "queryPreordered",
			treeID:      testonly.PreorderedLogTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_PREORDERED_LOG),
			storageTree: testonly.PreorderedLogTree,
			wantTree:    testonly.PreorderedLogTree,
		},
		{
			desc:        "queryFrozen",
			treeID:      frozenTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			storageTree: frozenTree,
			wantTree:    frozenTree,
		},
		{
			desc:        "sequenceFrozen",
			treeID:      frozenTree.TreeId,
			opts:        NewGetOpts(SequenceLog, trillian.TreeType_LOG),
			storageTree: frozenTree,
			wantTree:    frozenTree,
			wantErr:     true,
			code:        codes.PermissionDenied,
		},
		{
			desc:        "queueFrozen",
			treeID:      frozenTree.TreeId,
			opts:        NewGetOpts(QueueLog, trillian.TreeType_LOG),
			storageTree: frozenTree,
			wantTree:    frozenTree,
			wantErr:     true,
			code:        codes.PermissionDenied,
		},
		{
			desc:        "queryDraining",
			treeID:      drainingTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			storageTree: drainingTree,
			wantTree:    drainingTree,
		},
		{
			desc:        "sequenceDraining",
			treeID:      drainingTree.TreeId,
			opts:        NewGetOpts(SequenceLog, trillian.TreeType_LOG),
			storageTree: drainingTree,
			wantTree:    drainingTree,
		},
		{
			desc:        "queueDraining",
			treeID:      drainingTree.TreeId,
			opts:        NewGetOpts(QueueLog, trillian.TreeType_LOG),
			storageTree: drainingTree,
			wantTree:    drainingTree,
			wantErr:     true,
			code:        codes.PermissionDenied,
		},
		{
			desc:        "softDeleted",
			treeID:      softDeletedTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			storageTree: softDeletedTree,
			wantErr:     true, // Deleted = true makes the tree "invisible" for most RPCs
			code:        codes.NotFound,
		},
		{
			desc:     "treeInCtx",
			treeID:   logTree.TreeId,
			opts:     NewGetOpts(Query, trillian.TreeType_LOG),
			ctxTree:  logTree,
			wantTree: logTree,
		},
		{
			desc:        "wrongTreeInCtx",
			treeID:      logTree.TreeId,
			opts:        NewGetOpts(Query, trillian.TreeType_LOG),
			ctxTree:     frozenTree,
			storageTree: logTree,
			wantTree:    logTree,
			wantErr:     true,
			code:        codes.Internal,
		},
		{
			desc:     "beginErr",
			treeID:   logTree.TreeId,
			opts:     NewGetOpts(Query, trillian.TreeType_LOG),
			beginErr: errors.New("begin err"),
			wantErr:  true,
			code:     codes.Unknown,
		},
		{
			desc:    "getErr",
			treeID:  logTree.TreeId,
			opts:    NewGetOpts(Query, trillian.TreeType_LOG),
			getErr:  errors.New("get err"),
			wantErr: true,
			code:    codes.Unknown,
		},
		{
			desc:      "commitErr",
			treeID:    logTree.TreeId,
			opts:      NewGetOpts(Query, trillian.TreeType_LOG),
			commitErr: errors.New("commit err"),
			wantErr:   true,
			code:      codes.Unknown,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, test := range tests {
		ctx := NewContext(context.Background(), test.ctxTree)

		admin := storage.NewMockAdminStorage(ctrl)
		tx := storage.NewMockReadOnlyAdminTX(ctrl)
		admin.EXPECT().Snapshot(gomock.Any()).MaxTimes(1).Return(tx, test.beginErr)
		tx.EXPECT().GetTree(gomock.Any(), test.treeID).MaxTimes(1).Return(test.storageTree, test.getErr)
		tx.EXPECT().Close().MaxTimes(1).Return(nil)
		tx.EXPECT().Commit().MaxTimes(1).Return(test.commitErr)

		tree, err := GetTree(ctx, admin, test.treeID, test.opts)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTree() = (_, %q), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			if status.Code(err) != test.code {
				t.Errorf("%v: GetTree() = (_, %q), got ErrorCode: %v, want: %v", test.desc, err, status.Code(err), test.code)
			}
			continue
		}

		if !proto.Equal(tree, test.wantTree) {
			diff := cmp.Diff(tree, test.wantTree)
			t.Errorf("%v: post-GetTree diff:\n%v", test.desc, diff)
		}
	}
}

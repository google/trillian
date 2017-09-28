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

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
)

func TestDeletedTreeGC_Run(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test the following scenario:
	// * 1st iteration: tree1 is listed and hard-deleted
	// * Sleep
	// * 2nd iteration: ListTrees returns an empty slice, nothing gets deleted
	// * Sleep (ctx cancelled)
	//
	// DeletedTreeGC.Run() until ctx in cancelled. Since it always sleeps between iterations, we
	// make "timeSleep" cancel ctx the second time around to break the loop.

	tree1 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree1.TreeId = 1
	tree1.Deleted = true
	tree1.DeleteTime, _ = ptypes.TimestampProto(time.Date(2017, 9, 21, 10, 0, 0, 0, time.UTC))

	listTX1 := storage.NewMockReadOnlyAdminTX(ctrl)
	listTX2 := storage.NewMockReadOnlyAdminTX(ctrl)
	deleteTX1 := storage.NewMockAdminTX(ctrl)
	as := storage.NewMockAdminStorage(ctrl)

	ctx, cancel := context.WithCancel(context.Background())

	// Sequence Snapshot()/Begin() calls.
	// * 1st loop: Snapshot()/ListTrees() followed by Begin()/HardDeleteTree()
	// * 2nd loop: Snapshot()/ListTrees() only.
	lastTXCall := as.EXPECT().Snapshot(ctx).Return(listTX1, nil)
	lastTXCall = as.EXPECT().Begin(ctx).Return(deleteTX1, nil).After(lastTXCall)
	as.EXPECT().Snapshot(ctx).Return(listTX2, nil).After(lastTXCall)

	// 1st loop
	listTX1.EXPECT().ListTrees(ctx, true /* includeDeleted */).Return([]*trillian.Tree{tree1}, nil)
	listTX1.EXPECT().Close().Return(nil)
	listTX1.EXPECT().Commit().Return(nil)
	deleteTX1.EXPECT().HardDeleteTree(ctx, tree1.TreeId).Return(nil)
	deleteTX1.EXPECT().Close().Return(nil)
	deleteTX1.EXPECT().Commit().Return(nil)

	// 2nd loop
	listTX2.EXPECT().ListTrees(ctx, true /* includeDeleted */).Return(nil, nil)
	listTX2.EXPECT().Close().Return(nil)
	listTX2.EXPECT().Commit().Return(nil)

	defer func(now func() time.Time, sleep func(time.Duration)) {
		timeNow = now
		timeSleep = sleep
	}(timeNow, timeSleep)

	const deleteThreshold = 1 * time.Hour
	const runInterval = 3 * time.Second

	// now > tree1.DeleteTime + deleteThreshold, so tree1 gets deleted on first round
	now, _ := ptypes.Timestamp(tree1.DeleteTime)
	now = now.Add(deleteThreshold).Add(1 * time.Second)
	timeNow = func() time.Time { return now }

	calls := 0
	timeSleep = func(d time.Duration) {
		calls++
		if d < runInterval || d >= 2*runInterval {
			t.Errorf("Called time.Sleep(%v), want %v", d, runInterval)
		}
		if calls >= 2 {
			cancel()
		}
	}

	gc := &DeletedTreeGC{
		Admin:           as,
		DeleteThreshold: deleteThreshold,
		MinRunInterval:  runInterval,
	}
	gc.Run(ctx)
}

func TestDeletedTreeGC_RunOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tree1 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree1.TreeId = 1
	tree2 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree2.TreeId = 2
	tree2.Deleted = true
	tree2.DeleteTime, _ = ptypes.TimestampProto(time.Date(2017, 9, 21, 10, 0, 0, 0, time.UTC))
	tree3 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree3.TreeId = 3
	tree3.Deleted = true
	tree3.DeleteTime, _ = ptypes.TimestampProto(time.Date(2017, 9, 22, 11, 0, 0, 0, time.UTC))
	tree4 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree4.TreeId = 4
	tree4.Deleted = true
	tree4.DeleteTime, _ = ptypes.TimestampProto(time.Date(2017, 9, 23, 12, 0, 0, 0, time.UTC))
	tree5 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree5.TreeId = 5
	allTrees := []*trillian.Tree{tree1, tree2, tree3, tree4, tree5}

	tests := []struct {
		desc            string
		now             time.Time
		deleteThreshold time.Duration
		wantDeleted     []int64
	}{
		{
			desc:            "noDeletions",
			now:             time.Date(2017, 9, 28, 10, 0, 0, 0, time.UTC),
			deleteThreshold: 7 * 24 * time.Hour,
		},
		{
			desc:            "oneDeletion",
			now:             time.Date(2017, 9, 28, 11, 0, 0, 0, time.UTC),
			deleteThreshold: 7 * 24 * time.Hour,
			wantDeleted:     []int64{tree2.TreeId},
		},
		{
			desc:            "twoDeletions",
			now:             time.Date(2017, 9, 22, 12, 1, 0, 0, time.UTC),
			deleteThreshold: 1 * time.Hour,
			wantDeleted:     []int64{tree2.TreeId, tree3.TreeId},
		},
		{
			desc:            "threeDeletions",
			now:             time.Date(2017, 9, 23, 13, 30, 0, 0, time.UTC),
			deleteThreshold: 1 * time.Hour,
			wantDeleted:     []int64{tree2.TreeId, tree3.TreeId, tree4.TreeId},
		},
	}

	defer func(f func() time.Time) { timeNow = f }(timeNow)
	ctx := context.Background()
	for _, test := range tests {
		timeNow = func() time.Time { return test.now }

		as := storage.NewMockAdminStorage(ctrl)
		listTX := storage.NewMockReadOnlyAdminTX(ctrl)

		lastTXCall := as.EXPECT().Snapshot(ctx).Return(listTX, nil)
		listTX.EXPECT().ListTrees(ctx, true /* includeDeleted */).Return(allTrees, nil)
		listTX.EXPECT().Close().Return(nil)
		listTX.EXPECT().Commit().Return(nil)

		for _, id := range test.wantDeleted {
			deleteTX := storage.NewMockAdminTX(ctrl)
			lastTXCall = as.EXPECT().Begin(ctx).After(lastTXCall).Return(deleteTX, nil)
			deleteTX.EXPECT().HardDeleteTree(ctx, id).Return(nil)
			deleteTX.EXPECT().Close().Return(nil)
			deleteTX.EXPECT().Commit().Return(nil)
		}

		gc := &DeletedTreeGC{
			Admin:           as,
			DeleteThreshold: test.deleteThreshold,
			MinRunInterval:  1 * time.Second, // Doesn't matter for this test
		}
		gc.RunOnce(ctx)
	}
}

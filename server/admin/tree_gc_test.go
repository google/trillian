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
	"errors"
	"strings"
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

	NewDeletedTreeGC(as, deleteThreshold, runInterval, nil /* mf */).Run(ctx)
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

		gc := NewDeletedTreeGC(
			as, test.deleteThreshold, 1*time.Second /* minRunInterval */, nil /* mf */)
		switch count, err := gc.RunOnce(ctx); {
		case err != nil:
			t.Errorf("%v: RunOnce() returned err = %v", test.desc, err)
		case count != len(test.wantDeleted):
			t.Errorf("%v: RunOnce() = %v, want = %v", test.desc, count, len(test.wantDeleted))
		}
	}
}

func TestDeletedTreeGC_RunOnceErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deleteTime := time.Date(2017, 10, 25, 16, 0, 0, 0, time.UTC)
	deleteTimePB, err := ptypes.TimestampProto(deleteTime)
	if err != nil {
		t.Fatalf("TimestampProto(%v) returned err = %v", deleteTime, err)
	}
	logTree1 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree1.TreeId = 10
	logTree1.Deleted = true
	logTree1.DeleteTime = deleteTimePB
	logTree2 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree2.TreeId = 20
	logTree2.Deleted = true
	logTree2.DeleteTime = deleteTimePB
	mapTree := proto.Clone(testonly.MapTree).(*trillian.Tree)
	mapTree.TreeId = 30
	mapTree.Deleted = true
	mapTree.DeleteTime = deleteTimePB
	badTS := proto.Clone(testonly.LogTree).(*trillian.Tree)
	badTS.TreeId = 40
	badTS.Deleted = true
	// badTS.DeleteTime is nil

	// To simplify the test all trees are deleted and passed the deletion threshold.
	// Other aspects of RunOnce() are covered by TestDeletedTreeGC_RunOnce.
	deleteThreshold := 1 * time.Hour
	now := deleteTime.Add(2 * time.Hour)
	defer func(f func() time.Time) { timeNow = f }(timeNow)
	timeNow = func() time.Time { return now }

	tests := []struct {
		desc string

		// snapshotErr, listErr and snapshotCommitErr refer to the ListTrees() call.
		snapshotErr, listErr, snapshotCommitErr error
		// trees are the trees returned by ListTrees().
		trees []*trillian.Tree

		// beginErrs, deleteErrs, commitErrs and wantTreeIDs refer to the HardDeleteTree() calls
		// that follow ListTrees(). They must have the same size.
		beginErrs, deleteErrs, commitErrs []error
		wantTreeIDs                       []int64

		// wantCount is the count of successfully deleted trees.
		wantCount int
		// wantErrs defines which strings must be present in the resulting error.
		wantErrs []string
	}{
		{
			desc:        "snapshotErr",
			snapshotErr: errors.New("snapshot err"),
			wantErrs:    []string{"snapshot err"},
		},
		{
			desc:     "listErr",
			listErr:  errors.New("list err"),
			wantErrs: []string{"list err"},
		},
		{
			desc:              "snapshotCommitErr",
			trees:             []*trillian.Tree{logTree1, logTree2, mapTree},
			snapshotCommitErr: errors.New("commit err"),
			wantErrs:          []string{"commit err"},
		},
		{
			desc:        "beginErr",
			trees:       []*trillian.Tree{logTree1, logTree2},
			beginErrs:   []error{errors.New("begin err"), nil},
			deleteErrs:  []error{nil, nil},
			commitErrs:  []error{nil, nil},
			wantTreeIDs: []int64{logTree1.TreeId, logTree2.TreeId},
			wantCount:   1,
			wantErrs:    []string{"begin err"},
		},
		{
			desc:        "deleteErr",
			trees:       []*trillian.Tree{logTree1, logTree2},
			beginErrs:   []error{nil, nil},
			deleteErrs:  []error{errors.New("cannot delete logTree1"), nil},
			commitErrs:  []error{nil, nil},
			wantTreeIDs: []int64{logTree1.TreeId, logTree2.TreeId},
			wantCount:   1,
			wantErrs:    []string{"cannot delete logTree1"},
		},
		{
			desc:        "commitErr",
			trees:       []*trillian.Tree{logTree1, logTree2},
			beginErrs:   []error{nil, nil},
			deleteErrs:  []error{nil, nil},
			commitErrs:  []error{errors.New("commit err"), nil},
			wantTreeIDs: []int64{logTree1.TreeId, logTree2.TreeId},
			wantCount:   1,
			wantErrs:    []string{"commit err"},
		},
		{
			// logTree1 = delete successful
			// logTree2 = delete error
			// mapTree  = commit error
			// badTS    = timestamp parse error (no HardDeleteTree() call)
			desc:        "multipleErrors",
			trees:       []*trillian.Tree{logTree1, logTree2, mapTree, badTS},
			beginErrs:   []error{nil, nil, nil},
			deleteErrs:  []error{nil, errors.New("delete err"), nil},
			commitErrs:  []error{nil, nil, errors.New("commit err")},
			wantTreeIDs: []int64{logTree1.TreeId, logTree2.TreeId, mapTree.TreeId},
			wantCount:   1,
			wantErrs:    []string{"delete err", "commit err", "error parsing delete_time"},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		as := storage.NewMockAdminStorage(ctrl)

		listTX := storage.NewMockReadOnlyAdminTX(ctrl)
		lastTXCall := as.EXPECT().Snapshot(gomock.Any()).Return(listTX, test.snapshotErr)
		listTX.EXPECT().ListTrees(gomock.Any(), true /* includeDeleted */).AnyTimes().Return(test.trees, test.listErr)
		listTX.EXPECT().Close().AnyTimes().Return(nil)
		listTX.EXPECT().Commit().AnyTimes().Return(test.snapshotCommitErr)

		// Sanity check test setup
		if l1, l2, l3, l4 := len(test.beginErrs), len(test.deleteErrs), len(test.commitErrs), len(test.wantTreeIDs); l1 != l2 || l1 != l3 || l1 != l4 {
			t.Fatalf("%v: beginErrs, deleteErrs, commitErrs and wantTreeIDs have different lenghts: %v, %v, %v and %v", test.desc, l1, l2, l3, l4)
		}

		for i, beginErr := range test.beginErrs {
			deleteErr := test.deleteErrs[i]
			commitErr := test.commitErrs[i]
			id := test.wantTreeIDs[i]

			deleteTX := storage.NewMockAdminTX(ctrl)
			lastTXCall = as.EXPECT().Begin(gomock.Any()).Return(deleteTX, beginErr).After(lastTXCall)
			deleteTX.EXPECT().HardDeleteTree(gomock.Any(), id).AnyTimes().Return(deleteErr)
			deleteTX.EXPECT().Close().AnyTimes().Return(nil)
			deleteTX.EXPECT().Commit().AnyTimes().Return(commitErr)
		}

		gc := NewDeletedTreeGC(
			as, deleteThreshold, 1*time.Second /* minRunInterval */, nil /* mf */)
		count, err := gc.RunOnce(ctx)
		if err == nil {
			t.Errorf("%v: RunOnce() returned err = nil, want non-nil", test.desc)
			continue
		}

		if count != test.wantCount {
			t.Errorf("%v: RunOnce() = %v, want = %v", test.desc, count, test.wantCount)
		}

		failed := false
		for _, want := range test.wantErrs {
			if !strings.Contains(err.Error(), want) {
				if !failed {
					t.Errorf("%v: RunOnce() returned err = %v (see following errors)", test.desc, err)
					failed = true
				}
				t.Errorf("%v: RunOnce(): err doesn't contain %q", test.desc, want)
			}
		}
	}
}

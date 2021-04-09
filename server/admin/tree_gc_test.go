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

package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	tree1.DeleteTime = timestamppb.New(time.Date(2017, 9, 21, 10, 0, 0, 0, time.UTC))

	listTX1 := storage.NewMockReadOnlyAdminTX(ctrl)
	listTX2 := storage.NewMockReadOnlyAdminTX(ctrl)
	deleteTX1 := storage.NewMockAdminTX(ctrl)
	as := &testonly.FakeAdminStorage{
		TX:         []storage.AdminTX{deleteTX1},
		ReadOnlyTX: []storage.ReadOnlyAdminTX{listTX1, listTX2},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Sequence Snapshot()/ReadWriteTransaction() calls.
	// * 1st loop: Snapshot()/ListTrees() followed by ReadWriteTransaction()/HardDeleteTree()
	// * 2nd loop: Snapshot()/ListTrees() only.

	// 1st loop
	listTX1.EXPECT().ListTrees(gomock.Any(), true /* includeDeleted */).Return([]*trillian.Tree{tree1}, nil)
	listTX1.EXPECT().Close().Return(nil)
	listTX1.EXPECT().Commit().Return(nil)
	deleteTX1.EXPECT().HardDeleteTree(gomock.Any(), tree1.TreeId).Return(nil)
	deleteTX1.EXPECT().Close().Return(nil)
	deleteTX1.EXPECT().Commit().Return(nil)

	// 2nd loop
	listTX2.EXPECT().ListTrees(gomock.Any(), true /* includeDeleted */).Return(nil, nil)
	listTX2.EXPECT().Close().Return(nil)
	listTX2.EXPECT().Commit().Return(nil)

	defer func(now func() time.Time, sleep func(time.Duration)) {
		timeNow = now
		timeSleep = sleep
	}(timeNow, timeSleep)

	const deleteThreshold = 1 * time.Hour
	const runInterval = 3 * time.Second

	// now > tree1.DeleteTime + deleteThreshold, so tree1 gets deleted on first round
	now := tree1.DeleteTime.AsTime().Add(deleteThreshold).Add(1 * time.Second)
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
	tree2.DeleteTime = timestamppb.New(time.Date(2017, 9, 21, 10, 0, 0, 0, time.UTC))
	tree3 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree3.TreeId = 3
	tree3.Deleted = true
	tree3.DeleteTime = timestamppb.New(time.Date(2017, 9, 22, 11, 0, 0, 0, time.UTC))
	tree4 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	tree4.TreeId = 4
	tree4.Deleted = true
	tree4.DeleteTime = timestamppb.New(time.Date(2017, 9, 23, 12, 0, 0, 0, time.UTC))
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

		listTX := storage.NewMockReadOnlyAdminTX(ctrl)
		as := &testonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{listTX}}

		listTX.EXPECT().ListTrees(gomock.Any(), true /* includeDeleted */).Return(allTrees, nil)
		listTX.EXPECT().Close().Return(nil)
		listTX.EXPECT().Commit().Return(nil)

		for _, id := range test.wantDeleted {
			deleteTX := storage.NewMockAdminTX(ctrl)
			deleteTX.EXPECT().HardDeleteTree(gomock.Any(), id).Return(nil)
			deleteTX.EXPECT().Close().Return(nil)
			deleteTX.EXPECT().Commit().Return(nil)
			as.TX = append(as.TX, deleteTX)
		}

		gc := NewDeletedTreeGC(as, test.deleteThreshold, 1*time.Second /* minRunInterval */, nil /* mf */)
		switch count, err := gc.RunOnce(ctx); {
		case err != nil:
			t.Errorf("%v: RunOnce() returned err = %v", test.desc, err)
		case count != len(test.wantDeleted):
			t.Errorf("%v: RunOnce() = %v, want = %v", test.desc, count, len(test.wantDeleted))
		}
	}
}

// listTreesSpec specifies all parameters required to mock a ListTrees TX call.
type listTreesSpec struct {
	snapshotErr, listErr, commitErr error
	trees                           []*trillian.Tree
}

// hardDeleteTreeSpec specifies all parameters required to mock a HardDeleteTree TX call.
type hardDeleteTreeSpec struct {
	beginErr, deleteErr, commitErr error
	treeID                         int64
}

func TestDeletedTreeGC_RunOnceErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deleteTime := time.Date(2017, 10, 25, 16, 0, 0, 0, time.UTC)
	deleteTimePB := timestamppb.New(deleteTime)
	logTree1 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree1.TreeId = 10
	logTree1.Deleted = true
	logTree1.DeleteTime = deleteTimePB
	logTree2 := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree2.TreeId = 20
	logTree2.Deleted = true
	logTree2.DeleteTime = deleteTimePB
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

		listTrees      listTreesSpec
		hardDeleteTree []hardDeleteTreeSpec

		// wantCount is the count of successfully deleted trees.
		wantCount int
		// wantErrs defines which strings must be present in the resulting error.
		wantErrs []string
	}{
		{
			desc: "snapshotErr",
			listTrees: listTreesSpec{
				snapshotErr: errors.New("snapshot err"),
			},
			wantErrs: []string{"snapshot err"},
		},
		{
			desc: "listErr",
			listTrees: listTreesSpec{
				listErr: errors.New("list err"),
			},
			wantErrs: []string{"list err"},
		},
		{
			desc: "snapshotCommitErr",
			listTrees: listTreesSpec{
				commitErr: errors.New("commit err"),
				trees:     []*trillian.Tree{logTree1, logTree2},
			},
			wantErrs: []string{"commit err"},
		},
		{
			desc: "beginErr",
			listTrees: listTreesSpec{
				trees: []*trillian.Tree{logTree1, logTree2},
			},
			hardDeleteTree: []hardDeleteTreeSpec{
				{beginErr: errors.New("begin err")},
				{treeID: logTree2.TreeId},
			},
			wantCount: 1,
			wantErrs:  []string{"begin err"},
		},
		{
			desc: "deleteErr",
			listTrees: listTreesSpec{
				trees: []*trillian.Tree{logTree1, logTree2},
			},
			hardDeleteTree: []hardDeleteTreeSpec{
				{deleteErr: errors.New("cannot delete logTree1"), treeID: logTree1.TreeId},
				{treeID: logTree2.TreeId},
			},
			wantCount: 1,
			wantErrs:  []string{"cannot delete logTree1"},
		},
		{
			desc: "commitErr",
			listTrees: listTreesSpec{
				trees: []*trillian.Tree{logTree1, logTree2},
			},
			hardDeleteTree: []hardDeleteTreeSpec{
				{commitErr: errors.New("commit err"), treeID: logTree1.TreeId},
				{treeID: logTree2.TreeId},
			},
			wantCount: 1,
			wantErrs:  []string{"commit err"},
		},
		{
			// logTree1 = delete successful
			// logTree2 = delete error
			// badTS    = timestamp parse error (no HardDeleteTree() call)
			desc: "multipleErrors",
			listTrees: listTreesSpec{
				trees: []*trillian.Tree{logTree1, logTree2, badTS},
			},
			hardDeleteTree: []hardDeleteTreeSpec{
				{treeID: logTree1.TreeId},
				{deleteErr: errors.New("delete err"), treeID: logTree2.TreeId},
			},
			wantCount: 1,
			wantErrs:  []string{"delete err", "error parsing delete_time"},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			listTX := storage.NewMockReadOnlyAdminTX(ctrl)
			listTX.EXPECT().ListTrees(gomock.Any(), true /* includeDeleted */).AnyTimes().Return(test.listTrees.trees, test.listTrees.listErr)
			listTX.EXPECT().Close().AnyTimes().Return(nil)
			listTX.EXPECT().Commit().AnyTimes().Return(test.listTrees.commitErr)

			as := &testonly.FakeAdminStorage{
				ReadOnlyTX: []storage.ReadOnlyAdminTX{listTX},
			}
			if test.listTrees.snapshotErr != nil {
				as.SnapshotErr = append(as.SnapshotErr, test.listTrees.snapshotErr)
			}

			for _, hardDeleteTree := range test.hardDeleteTree {
				deleteTX := storage.NewMockAdminTX(ctrl)

				if hardDeleteTree.beginErr != nil {
					as.TXErr = append(as.TXErr, hardDeleteTree.beginErr)
				} else {
					as.TX = append(as.TX, deleteTX)
				}

				if hardDeleteTree.treeID != 0 {
					deleteTX.EXPECT().HardDeleteTree(gomock.Any(), hardDeleteTree.treeID).AnyTimes().Return(hardDeleteTree.deleteErr)
				}
				deleteTX.EXPECT().Close().AnyTimes().Return(nil)
				deleteTX.EXPECT().Commit().AnyTimes().Return(hardDeleteTree.commitErr)
			}

			gc := NewDeletedTreeGC(as, deleteThreshold, 1*time.Second /* minRunInterval */, nil /* mf */)
			count, err := gc.RunOnce(ctx)
			if err == nil {
				t.Fatalf("%v: RunOnce() returned err = nil, want non-nil", test.desc)
			}
			if count != test.wantCount {
				t.Errorf("%v: RunOnce() = %v, want = %v", test.desc, count, test.wantCount)
			}
			for _, want := range test.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%v: RunOnce() returned err = %q, want substring %q", test.desc, err, want)
				}
			}
		})
	}
}

// Copyright 2016 Google Inc. All Rights Reserved.
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

package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election/stub"
)

func defaultLogOperationInfo(registry extension.Registry) LogOperationInfo {
	return LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		NumWorkers:  1,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
}

func TestLogOperationManagerSnapshotFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().Snapshot(gomock.Any()).Return(nil, errors.New("TX"))

	registry := extension.Registry{
		LogStorage: fakeStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs(gomock.Any()).Return(nil, errors.New("getactivelogs"))
	mockTx.EXPECT().Close().Return(nil)
	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	registry := extension.Registry{
		LogStorage: fakeStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

func TestLogOperationManagerCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs(gomock.Any()).Return([]int64{}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("commit"))
	mockTx.EXPECT().Close().Return(nil)
	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	registry := extension.Registry{
		LogStorage: fakeStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

type logOpInfoMatcher struct {
	BatchSize int
}

func (l logOpInfoMatcher) Matches(x interface{}) bool {
	o, ok := x.(*LogOperationInfo)
	if !ok {
		return false
	}
	return o.BatchSize == l.BatchSize
}

func (l logOpInfoMatcher) String() string {
	return fmt.Sprintf("has batchSize %d", l.BatchSize)
}

// Set up some log IDs in mock storage, together with some special values:
//  logID==98: fail the GetTree() operation
//  logID==99: return a tree with no DisplayName
func setupLogIDs(ctrl *gomock.Controller, logNames map[int64]string) (*storage.MockLogStorage, *storage.MockAdminStorage) {
	ids := make([]int64, 0, len(logNames))
	for id := range logNames {
		ids = append(ids, id)
	}

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs(gomock.Any()).AnyTimes().Return(ids, nil)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockTx.EXPECT().Close().AnyTimes().Return(nil)
	fakeStorage.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(mockTx, nil)

	mockAdmin := storage.NewMockAdminStorage(ctrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(ctrl)
	for id, name := range logNames {
		mockAdminTx.EXPECT().GetTree(gomock.Any(), id).AnyTimes().Return(&trillian.Tree{TreeId: id, DisplayName: name}, nil)
	}
	mockAdminTx.EXPECT().GetTree(gomock.Any(), int64(98)).AnyTimes().Return(nil, errors.New("failedGetTree"))
	mockAdminTx.EXPECT().GetTree(gomock.Any(), int64(99)).AnyTimes().Return(&trillian.Tree{TreeId: 99}, nil)
	mockAdminTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockAdminTx.EXPECT().Close().AnyTimes().Return(nil)
	mockAdmin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(mockAdminTx, nil)

	return fakeStorage, mockAdmin
}

func TestLogOperationManagerPassesIDs(t *testing.T) {
	ctx := context.Background()
	logID1 := int64(451)
	logID2 := int64(145)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage, mockAdmin := setupLogIDs(ctrl, map[int64]string{451: "LogID1", 145: "LogID2"})
	registry := extension.Registry{
		LogStorage:   fakeStorage,
		AdminStorage: mockAdmin,
	}

	mockLogOp := NewMockLogOperation(ctrl)
	infoMatcher := logOpInfoMatcher{50}
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher)

	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

func TestHeldInfo(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeStorage, mockAdmin := setupLogIDs(ctrl, map[int64]string{1: "one", 2: "two"})
	registry := extension.Registry{LogStorage: fakeStorage, AdminStorage: mockAdmin}
	mockLogOp := NewMockLogOperation(ctrl)
	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	var tests = []struct {
		in   []int64
		want string
	}{
		{in: []int64{}, want: "master for:"},
		{in: []int64{1, 2}, want: "master for: one two"},
		{in: []int64{2, 1}, want: "master for: one two"},
		{in: []int64{2, 1, 2}, want: "master for: one two two"},
		{in: []int64{2, 1, 99}, want: "master for: <log-99> one two"},
		{in: []int64{2, 1, 98, 1}, want: "master for: <err> one one two"},
	}
	for _, test := range tests {
		got := lom.heldInfo(ctx, test.in)
		if got != test.want {
			t.Errorf("lom.HeldInfo(%+v)=%q; want %q", test.in, got, test.want)
		}
	}
}

func TestMasterFor(t *testing.T) {
	ctx := context.Background()
	firstIDs := []int64{1, 2, 3, 4}
	allIDs := []int64{1, 2, 3, 4, 5, 6}

	var tests = []struct {
		desc    string
		factory election.Factory
		want1   []int64
		want2   []int64
	}{
		{desc: "no-factory", factory: nil, want1: firstIDs, want2: allIDs},
		{desc: "noop-factory", factory: election.NoopFactory{InstanceID: "test"}, want1: firstIDs, want2: allIDs},
		{desc: "master-for-even", factory: masterForEvenFactory{}, want1: []int64{2, 4}, want2: []int64{2, 4, 6}},
		{desc: "failure-factory", factory: failureFactory{}, want1: nil, want2: nil},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			testCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			registry := extension.Registry{ElectionFactory: test.factory}
			info := LogOperationInfo{
				Registry:   registry,
				TimeSource: util.SystemTimeSource{},
			}
			lom := NewLogOperationManager(info, nil)

			// Check mastership twice, to give the election threads a chance to get started and report.
			lom.masterFor(testCtx, firstIDs)
			time.Sleep(2 * election.MinMasterCheckInterval)
			logIDs, err := lom.masterFor(testCtx, firstIDs)
			if !reflect.DeepEqual(logIDs, test.want1) {
				t.Fatalf("masterFor(factory=%T)=%v,%v; want %v,_", test.factory, logIDs, err, test.want1)
			}
			// Now add extra IDs and re-check.
			lom.masterFor(testCtx, allIDs)
			time.Sleep(2 * election.MinMasterCheckInterval)
			logIDs, err = lom.masterFor(testCtx, allIDs)
			if !reflect.DeepEqual(logIDs, test.want2) {
				t.Fatalf("masterFor(factory=%T)=%v,%v; want %v,_", test.factory, logIDs, err, test.want2)
			}
		})
	}
}

type masterForEvenFactory struct{}

func (m masterForEvenFactory) NewElection(ctx context.Context, treeID int64) (election.MasterElection, error) {
	isMaster := (treeID % 2) == 0
	return stub.NewMasterElection(isMaster, nil), nil
}

type failureFactory struct{}

func (ff failureFactory) NewElection(ctx context.Context, treeID int64) (election.MasterElection, error) {
	return nil, errors.New("injected failure")
}

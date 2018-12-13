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
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring/testonly"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util/clock"
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
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher).Return(1, nil)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher).Return(0, nil)

	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

func TestLogOperationManagerExecutePassError(t *testing.T) {
	ctx := context.Background()
	logID1 := int64(451)
	logID2 := int64(145)
	logID1Label := strconv.FormatInt(logID1, 10)
	logID2Label := strconv.FormatInt(logID2, 10)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage, mockAdmin := setupLogIDs(ctrl, map[int64]string{451: "LogID1", 145: "LogID2"})
	registry := extension.Registry{
		LogStorage:   fakeStorage,
		AdminStorage: mockAdmin,
	}

	mockLogOp := NewMockLogOperation(ctrl)
	infoMatcher := logOpInfoMatcher{50}
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher).Return(1, nil)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher).Return(0, errors.New("test error"))

	// Do not run this test in parallel with any other that affects the signingRuns or failedSigningRuns counters.
	log1SigningRuns := testonly.NewCounterSnapshot(signingRuns, logID1Label)
	log1FailedSigningRuns := testonly.NewCounterSnapshot(failedSigningRuns, logID1Label)
	log2SigningRuns := testonly.NewCounterSnapshot(signingRuns, logID2Label)
	log2FailedSigningRuns := testonly.NewCounterSnapshot(failedSigningRuns, logID2Label)

	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)
	lom.OperationSingle(ctx)

	// Check that logID1 has 1 successful signing run, 0 failed.
	if want, got := 1, int(log1SigningRuns.Delta()); want != got {
		t.Errorf("want signingRuns[logID1] == %d, got %d", want, got)
	}
	if want, got := 0, int(log1FailedSigningRuns.Delta()); want != got {
		t.Errorf("want signingRuns[logID2] == %d, got %d", want, got)
	}
	// Check that logID2 has 0 successful signing runs, 1 failed.
	if want, got := 0, int(log2SigningRuns.Delta()); want != got {
		t.Errorf("want failedSigningRuns[logID1] == %d, got %d", want, got)
	}
	if want, got := 1, int(log2FailedSigningRuns.Delta()); want != got {
		t.Errorf("want failedSigningRuns[logID2] == %d, got %d", want, got)
	}
}

func TestLogOperationManagerOperationLoopPassesIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logID1 := int64(451)
	logID2 := int64(145)

	var logCount int64

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage, mockAdmin := setupLogIDs(ctrl, map[int64]string{451: "LogID1", 145: "LogID2"})
	registry := extension.Registry{
		LogStorage:   fakeStorage,
		AdminStorage: mockAdmin,
	}

	mockLogOp := NewMockLogOperation(ctrl)
	infoMatcher := logOpInfoMatcher{50}
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher).Do(func(_ context.Context, _ int64, _ *LogOperationInfo) {
		if atomic.AddInt64(&logCount, 1) == 2 {
			cancel()
		}
	}).Return(1, nil)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher).Do(func(_ context.Context, _ int64, _ *LogOperationInfo) {
		if atomic.AddInt64(&logCount, 1) == 2 {
			cancel()
		}
	}).Return(0, nil)

	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)
	lom.OperationLoop(ctx)
}

func TestLogOperationManagerOperationLoopExecutePassError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logID1 := int64(451)
	logID2 := int64(145)
	logID1Label := strconv.FormatInt(logID1, 10)
	logID2Label := strconv.FormatInt(logID2, 10)

	var logCount int64

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage, mockAdmin := setupLogIDs(ctrl, map[int64]string{451: "LogID1", 145: "LogID2"})
	registry := extension.Registry{
		LogStorage:   fakeStorage,
		AdminStorage: mockAdmin,
	}

	mockLogOp := NewMockLogOperation(ctrl)
	infoMatcher := logOpInfoMatcher{50}
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher).Do(func(_ context.Context, _ int64, _ *LogOperationInfo) {
		if atomic.AddInt64(&logCount, 1) == 2 {
			cancel()
		}
	}).Return(1, nil)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher).Do(func(_ context.Context, _ int64, _ *LogOperationInfo) {
		if atomic.AddInt64(&logCount, 1) == 2 {
			cancel()
		}
	}).Return(0, errors.New("test error"))

	// Do not run this test in parallel with any other that affects the signingRuns or failedSigningRuns counters.
	log1SigningRuns := testonly.NewCounterSnapshot(signingRuns, logID1Label)
	log1FailedSigningRuns := testonly.NewCounterSnapshot(failedSigningRuns, logID1Label)
	log2SigningRuns := testonly.NewCounterSnapshot(signingRuns, logID2Label)
	log2FailedSigningRuns := testonly.NewCounterSnapshot(failedSigningRuns, logID2Label)

	info := defaultLogOperationInfo(registry)
	lom := NewLogOperationManager(info, mockLogOp)
	lom.OperationLoop(ctx)

	// Check that logID1 has 1 successful signing run, 0 failed.
	if want, got := 1, int(log1SigningRuns.Delta()); want != got {
		t.Errorf("want signingRuns[logID1] == %d, got %d", want, got)
	}
	if want, got := 0, int(log1FailedSigningRuns.Delta()); want != got {
		t.Errorf("want signingRuns[logID2] == %d, got %d", want, got)
	}
	// Check that logID2 has 0 successful signing runs, 1 failed.
	if want, got := 0, int(log2SigningRuns.Delta()); want != got {
		t.Errorf("want failedSigningRuns[logID1] == %d, got %d", want, got)
	}
	if want, got := 1, int(log2FailedSigningRuns.Delta()); want != got {
		t.Errorf("want failedSigningRuns[logID2] == %d, got %d", want, got)
	}
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
				TimeSource: clock.System,
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

func (m masterForEvenFactory) NewElection(ctx context.Context, treeID string) (election.MasterElection, error) {
	id, err := strconv.ParseInt(treeID, 10, 64)
	if err != nil {
		return nil, err
	}
	isMaster := (id % 2) == 0
	return stub.NewMasterElection(isMaster, nil), nil
}

type failureFactory struct{}

func (ff failureFactory) NewElection(ctx context.Context, treeID string) (election.MasterElection, error) {
	return nil, errors.New("injected failure")
}

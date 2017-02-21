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
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
)

func TestLogOperationManagerSnapshotFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(nil, errors.New("TX"))

	mockRegistry := extension.NewMockRegistry(ctrl)
	mockRegistry.EXPECT().GetLogStorage().Return(mockStorage, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockRegistry, 50, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return(nil, errors.New("getactivelogs"))
	mockTx.EXPECT().Close().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	mockRegistry := extension.NewMockRegistry(ctrl)
	mockRegistry.EXPECT().GetLogStorage().Return(mockStorage, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockRegistry, 50, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]int64{}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("commit"))
	mockTx.EXPECT().Close().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	mockRegistry := extension.NewMockRegistry(ctrl)
	mockRegistry.EXPECT().GetLogStorage().Return(mockStorage, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockRegistry, 50, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

type logOpMgrContextMatcher struct {
	batchSize int
}

func (l logOpMgrContextMatcher) Matches(x interface{}) bool {
	o, ok := x.(LogOperationManagerContext)
	if !ok {
		return false
	}
	return o.batchSize == l.batchSize
}

func (l logOpMgrContextMatcher) String() string {
	return fmt.Sprintf("has batchSize %d", l.batchSize)
}

func TestLogOperationManagerPassesIDs(t *testing.T) {
	logID1 := int64(451)
	logID2 := int64(145)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]int64{logID1, logID2}, nil)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockTx.EXPECT().Close().AnyTimes().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	mockRegistry := extension.NewMockRegistry(ctrl)
	mockRegistry.EXPECT().GetLogStorage().Return(mockStorage, nil)

	mockLogOp := NewMockLogOperation(ctrl)
	mockLogOp.EXPECT().ExecutePass([]int64{logID1, logID2}, logOpMgrContextMatcher{50})

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockRegistry, 50, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

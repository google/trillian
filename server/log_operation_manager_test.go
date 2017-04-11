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
)

func TestLogOperationManagerSnapshotFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(nil, errors.New("TX"))

	registry := extension.Registry{
		LogStorage: mockStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		NumWorkers:  1,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockReadOnlyLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return(nil, errors.New("getactivelogs"))
	mockTx.EXPECT().Close().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Snapshot(gomock.Any()).Return(mockTx, nil)

	registry := extension.Registry{
		LogStorage: mockStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		NumWorkers:  1,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
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

	registry := extension.Registry{
		LogStorage: mockStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := context.Background()
	info := LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		NumWorkers:  1,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
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

func TestLogOperationManagerPassesIDs(t *testing.T) {
	ctx := context.Background()
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

	registry := extension.Registry{
		LogStorage: mockStorage,
	}

	mockLogOp := NewMockLogOperation(ctrl)
	infoMatcher := logOpInfoMatcher{50}
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID1, infoMatcher)
	mockLogOp.EXPECT().ExecutePass(gomock.Any(), logID2, infoMatcher)

	info := LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		NumWorkers:  1,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
	lom := NewLogOperationManager(info, mockLogOp)

	lom.OperationSingle(ctx)
}

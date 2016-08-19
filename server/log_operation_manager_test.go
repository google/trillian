package server

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

func TestLogOperationManagerBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	mockLogOp := NewMockLogOperation(ctrl)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]trillian.LogID{}, errors.New("getactivelogs"))
	mockTx.EXPECT().Rollback().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]trillian.LogID{}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("commit"))
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

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
	logID1 := trillian.LogID{TreeID: 451, LogID: []byte("id")}
	logID2 := trillian.LogID{TreeID: 145, LogID: []byte("id2")}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]trillian.LogID{logID1, logID2}, nil)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)
	mockLogOp.EXPECT().ExecutePass([]trillian.LogID{logID1, logID2}, logOpMgrContextMatcher{50}).Return(false)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

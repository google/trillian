package server

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

func TestLogOperationManagerBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]int64{}, errors.New("getactivelogs"))
	mockTx.EXPECT().Rollback().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

func TestLogOperationManagerCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]int64{}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("commit"))
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

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

	mockTx := storage.NewMockLogTX(ctrl)
	mockTx.EXPECT().GetActiveLogIDs().Return([]int64{logID1, logID2}, nil)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockLogOp := NewMockLogOperation(ctrl)
	mockLogOp.EXPECT().ExecutePass([]int64{logID1, logID2}, logOpMgrContextMatcher{50}).Return(false)

	ctx := util.NewLogContext(context.Background(), -1)
	lom := NewLogOperationManagerForTest(ctx, mockStorageProviderForSequencer(mockStorage), 50, time.Second, time.Second, fakeTimeSource, mockLogOp)

	lom.OperationLoop()
}

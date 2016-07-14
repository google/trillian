package server

import (
	"errors"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/stretchr/testify/mock"
)

type mockLogOperation struct {
	mock.Mock
}

func (d mockLogOperation) Name() string {
	args := d.Called()
	return args.String(0)
}

func (d mockLogOperation) ExecutePass(logIDs []trillian.LogID, context LogOperationManagerContext) bool {
	args := d.Called(logIDs, context)
	return args.Bool(0)
}

func TestLogOperationManagerBeginFails(t *testing.T) {
	mockTx := new(storage.MockLogTX)
	mockStorage := new(storage.MockLogStorage)
	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))

	mockLogOp := new(mockLogOperation)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, fakeTimeSource, *mockLogOp)

	lom.OperationLoop()

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
	mockLogOp.AssertExpectations(t)
}

func TestLogOperationManagerGetLogsFails(t *testing.T) {
	mockTx := new(storage.MockLogTX)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{}, errors.New("getactivelogs"))
	mockTx.On("Rollback").Return(nil)
	mockStorage := new(storage.MockLogStorage)
	mockStorage.On("Begin").Return(mockTx, nil)

	mockLogOp := new(mockLogOperation)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, fakeTimeSource, *mockLogOp)

	lom.OperationLoop()

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
	mockLogOp.AssertExpectations(t)
}

func TestLogOperationManagerCommitFails(t *testing.T) {
	mockTx := new(storage.MockLogTX)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{}, nil)
	mockTx.On("Commit").Return(errors.New("commit"))
	mockStorage := new(storage.MockLogStorage)
	mockStorage.On("Begin").Return(mockTx, nil)

	mockLogOp := new(mockLogOperation)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, fakeTimeSource, *mockLogOp)

	lom.OperationLoop()

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
	mockLogOp.AssertExpectations(t)
}

func TestLogOperationManagerPassesIDs(t *testing.T) {
	logID1 := trillian.LogID{TreeID: 451, LogID: []byte("id")}
	logID2 := trillian.LogID{TreeID: 145, LogID: []byte("id2")}

	mockTx := new(storage.MockLogTX)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{logID1, logID2}, nil)
	mockTx.On("Commit").Return(nil)
	mockStorage := new(storage.MockLogStorage)
	mockStorage.On("Begin").Return(mockTx, nil)

	mockLogOp := new(mockLogOperation)
	mockLogOp.On("ExecutePass", []trillian.LogID{logID1, logID2},
		mock.MatchedBy(func(other LogOperationManagerContext) bool {
			return other.batchSize == 50
		})).Return(false)

	done := make(chan struct{})
	lom := NewLogOperationManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Second, fakeTimeSource, *mockLogOp)

	lom.OperationLoop()

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
	mockLogOp.AssertExpectations(t)
}

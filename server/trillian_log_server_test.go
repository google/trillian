package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
)

var logId1 = int64(1)
var logId2 = int64(2)
var leaf0Request = trillian.GetLeavesByIndexRequest{LogId: &logId1, LeafIndex: []int64{0}}
var leaf0Minus2Request = trillian.GetLeavesByIndexRequest{LogId: &logId1, LeafIndex: []int64{0, -2}}
var leaf03Request = trillian.GetLeavesByIndexRequest{LogId: &logId1, LeafIndex: []int64{0, 3}}
var leaf0Log2Request = trillian.GetLeavesByIndexRequest{LogId: &logId2, LeafIndex: []int64{0}}

var leaf1 = trillian.LogLeaf{SequenceNumber: 1, Leaf: trillian.Leaf{LeafHash: []byte("hash"), LeafValue: []byte("value"), ExtraData: []byte("extra")}}
var leaf3 = trillian.LogLeaf{SequenceNumber: 3, Leaf: trillian.Leaf{LeafHash: []byte("hash3"), LeafValue: []byte("value3"), ExtraData: []byte("extra3")}}
var expectedLeaf1 = trillian.LeafProto{LeafHash: []byte("hash"), LeafData: []byte("value"), ExtraData: []byte("extra")}
var expectedLeaf3 = trillian.LeafProto{LeafHash: []byte("hash3"), LeafData: []byte("value3"), ExtraData: []byte("extra3")}

func mockStorageProviderfunc(mockStorage storage.LogStorage) LogStorageProviderFunc {
	return func(id int64) (storage.LogStorage, error) {
		if id == 1 {
			return mockStorage, nil
		} else {
			return nil, fmt.Errorf("BADLOGID: ", id)
		}
	}
}

func TestGetLeavesByIndexInvalidIndexRejected(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Minus2Request)

	if err != nil || *resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatalf("Returned non app level error response for negative leaf index")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexBeginFailsCausesError(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexStorageError(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByIndex", []int64{0}).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
	mockTx.On("Rollback").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong response when storage get leaves failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexInvalidLogId(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Log2Request)

	if err == nil || !strings.Contains(err.Error(), "BADLOGID") {
		t.Fatalf("Got wrong error response for unknown log id: %v", err)
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexCommitFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByIndex", []int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.On("Commit").Return(errors.New("Bang!"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil {
		t.Fatalf("Returned OK when commit failed: %v", err)
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndex(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByIndex", []int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.On("Commit").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v")
	}

	if len(resp.Leaves) != 1 || !proto.Equal(resp.Leaves[0], &expectedLeaf1) {
		t.Fatalf("Expected leaf: %v but got: %v", expectedLeaf1, resp.Leaves[0])
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexMultiple(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByIndex", []int64{0, 3}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.On("Commit").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf03Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v")
	}

	if len(resp.Leaves) != 2 {
		t.Fatalf("Expected two leaves but got %d", len(resp.Leaves))
	}

	if !proto.Equal(resp.Leaves[0], &expectedLeaf1) {
		t.Fatalf("Expected leaf1: %v but got: %v", expectedLeaf1, resp.Leaves[0])
	}

	if !proto.Equal(resp.Leaves[1], &expectedLeaf3) {
		t.Fatalf("Expected leaf3: %v but got: %v", expectedLeaf3, resp.Leaves[0])
	}

	mockStorage.AssertExpectations(t)
}

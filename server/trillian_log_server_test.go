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

var leaf0 = trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: []byte("hash"), LeafValue: []byte("value"), ExtraData: []byte("extra")}}
var leaf1 = trillian.LogLeaf{SequenceNumber: 1, Leaf: trillian.Leaf{LeafHash: []byte("hash"), LeafValue: []byte("value"), ExtraData: []byte("extra")}}
var leaf3 = trillian.LogLeaf{SequenceNumber: 3, Leaf: trillian.Leaf{LeafHash: []byte("hash3"), LeafValue: []byte("value3"), ExtraData: []byte("extra3")}}
var expectedLeaf1 = trillian.LeafProto{LeafHash: []byte("hash"), LeafData: []byte("value"), ExtraData: []byte("extra")}
var expectedLeaf3 = trillian.LeafProto{LeafHash: []byte("hash3"), LeafData: []byte("value3"), ExtraData: []byte("extra3")}

var queueRequest0 = trillian.QueueLeavesRequest{LogId: &logId1, Leaves: []*trillian.LeafProto{&expectedLeaf1}}
var queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: &logId2, Leaves: []*trillian.LeafProto{&expectedLeaf1}}
var queueRequestEmpty = trillian.QueueLeavesRequest{LogId: &logId1, Leaves: []*trillian.LeafProto{}}

var getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: &logId1}
var getLogRootRequest2 = trillian.GetLatestSignedLogRootRequest{LogId: &logId2}
var signedRoot1 = trillian.SignedLogRoot{TimestampNanos: proto.Int64(987654321), RootHash: []byte("A NICE HASH"), TreeSize: proto.Int64(7)}

var getByHashRequest1 = trillian.GetLeavesByHashRequest{LogId: &logId1, LeafHash: [][]byte{ []byte("test"), []byte("data")}}
var getByHashRequestBadHash = trillian.GetLeavesByHashRequest{LogId: &logId1, LeafHash: [][]byte{ []byte(""), []byte("data")}}
var getByHashRequest2 = trillian.GetLeavesByHashRequest{LogId: &logId2, LeafHash: [][]byte{ []byte("test"), []byte("data")}}

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
	mockTx.On("Open").Return(true)
	mockTx.On("Rollback").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong response when storage get leaves failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByIndexInvalidLogId(t *testing.T) {
	test := newCommitFailsTest("GetLeavesByIndex",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error { _, err := s.GetLeavesByIndex(context.Background(), &leaf0Log2Request); return err })

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByIndexCommitFails(t *testing.T) {
	test := newCommitFailsTest("GetLeavesByIndex",
		func(t *storage.MockLogTX) { t.On("GetLeavesByIndex", []int64{0}).Return([]trillian.LogLeaf{leaf1}, nil) },
		func(s *TrillianLogServer) error { _, err := s.GetLeavesByIndex(context.Background(), &leaf0Request) ; return err })

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByIndex(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByIndex", []int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.On("Commit").Return(nil)
	mockTx.On("Open").Return(false)

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
	mockTx.On("Open").Return(false)

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

func TestQueueLeavesStorageError(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("QueueLeaves", []trillian.LogLeaf{leaf0}).Return(errors.New("STORAGE"))
	mockTx.On("Open").Return(true)
	mockTx.On("Rollback").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.QueueLeaves(context.Background(), &queueRequest0)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong response when storage get leaves failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestQueueLeavesInvalidLogId(t *testing.T) {
	test := newCommitFailsTest("QueueLeaves",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error { _, err := s.QueueLeaves(context.Background(), &queueRequest0Log2); return err })

	test.executeInvalidLogIDTest(t)
}

func TestQueueLeavesCommitFails(t *testing.T) {
	test := newCommitFailsTest("QueueLeaves",
		func(t *storage.MockLogTX) { t.On("QueueLeaves", []trillian.LogLeaf{leaf0}).Return(nil) },
		func(s *TrillianLogServer) error { _, err := s.QueueLeaves(context.Background(), &queueRequest0) ; return err })

	test.executeCommitFailsTest(t)
}

func TestQueueLeaves(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("QueueLeaves", []trillian.LogLeaf{leaf0}).Return(nil)
	mockTx.On("Commit").Return(nil)
	mockTx.On("Open").Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.QueueLeaves(context.Background(), &queueRequest0)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v")
	}

	mockStorage.AssertExpectations(t)
}

func TestQueueLeavesNoLeavesRejected(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.QueueLeaves(context.Background(), &queueRequestEmpty)

	if err != nil || *resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatalf("Allowed zero leaves to be queued")
	}

	mockStorage.AssertExpectations(t)
}

func TestQueueLeavesBeginFailsCausesError(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.QueueLeaves(context.Background(), &queueRequest0)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLatestSignedLogRootBeginFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLatestSignedLogRootStorageFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(trillian.SignedLogRoot{}, errors.New("STORAGE"))
	mockTx.On("Rollback").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong error response when storage failed: %v", err)
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLatestSignedLogRootCommitFails(t *testing.T) {
	test := newCommitFailsTest("LatestSignedLogRoot",
		func(t *storage.MockLogTX) { t.On("LatestSignedLogRoot").Return(trillian.SignedLogRoot{}, nil) },
		func(s *TrillianLogServer) error { _, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1); return err })

	test.executeCommitFailsTest(t)
}

func TestGetLatestSignedLogRootInvalidLogId(t *testing.T) {
	test := newCommitFailsTest("LatestSignedLogRoot",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error { _, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest2); return err })

	test.executeInvalidLogIDTest(t)
}

func TestGetLatestSignedLogRoot(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("LatestSignedLogRoot").Return(signedRoot1, nil)
	mockTx.On("Commit").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err != nil {
		t.Fatalf("Failed to get log root: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", *resp.Status.StatusCode)
	}

	if !proto.Equal(&signedRoot1, resp.SignedLogRoot) {
		t.Fatalf("Log root proto mismatch:\n%v\n%v", signedRoot1, resp.SignedLogRoot)
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByHashInvalidHash(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	// This request includes an empty hash, which isn't allowed
	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequestBadHash)

	// Should have succeeded at RPC level
	if err != nil {
		t.Fatalf("Request failed with unexpected error: %v", err)
	}

	// And failed at app level
	if expected, got := trillian.TrillianApiStatusCode_ERROR, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level error status but got: %v", *resp.Status.StatusCode)
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByHashBeginFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}

	mockStorage.AssertExpectations(t)
}

func TestGetLeavesByHashStorageFails(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByHash", []trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
	mockTx.On("Rollback").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong error response when storage failed: %v", err)
	}

	mockStorage.AssertExpectations(t)
}

func TestLeavesByHashCommitFails(t *testing.T) {
	test := newCommitFailsTest("GetLeavesByHash",
		func(t *storage.MockLogTX) { t.On("GetLeavesByHash", []trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{}, nil) },
		func(s *TrillianLogServer) error { _, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1) ; return err })

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByHashInvalidLogId(t *testing.T) {
	test := newCommitFailsTest("GetLeavesByHash",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error { _, err := s.GetLeavesByHash(context.Background(), &getByHashRequest2); return err })

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByHash(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetLeavesByHash", []trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.On("Commit").Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err != nil {
		t.Fatalf("Got error trying to get leaves by hash: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, *resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", *resp.Status.StatusCode)
	}

	if len(resp.Leaves) != 2 || !proto.Equal(resp.Leaves[0], &expectedLeaf1) || !proto.Equal(resp.Leaves[1], &expectedLeaf3) {
		t.Fatalf("Expected leaves %v and %v but got: %v", expectedLeaf1, expectedLeaf3, resp.Leaves)
	}

	mockStorage.AssertExpectations(t)
}

type prepareMockTXFunc func(*storage.MockLogTX)
type makeRpcFunc func(*TrillianLogServer) error

type commitFailsTest struct {
	operation string
	prepareTx prepareMockTXFunc
	makeRpc   makeRpcFunc
}

func newCommitFailsTest(operation string, prepareTx prepareMockTXFunc, makeRpc makeRpcFunc) *commitFailsTest {
	return &commitFailsTest{operation, prepareTx, makeRpc}
}

func (c *commitFailsTest) executeCommitFailsTest(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	c.prepareTx(mockTx)
	mockTx.On("Commit").Return(errors.New("Bang!"))
	mockTx.On("Open").Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	err := c.makeRpc(server)

	if err == nil {
		t.Fatalf("Returned OK when commit failed: %s: %v", c.operation, err)
	}

	mockStorage.AssertExpectations(t)
}

func (c *commitFailsTest) executeInvalidLogIDTest(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	// Make a request for a nonexistent log id
	err := c.makeRpc(server)

	if err == nil || !strings.Contains(err.Error(), "BADLOGID") {
		t.Fatalf("Returned wrong error response for nonexistent log: %v", err)
	}

	mockStorage.AssertExpectations(t)
}
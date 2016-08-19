package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
)

var logId1 = int64(1)
var logId2 = int64(2)
var leaf0Request = trillian.GetLeavesByIndexRequest{LogId: logId1, LeafIndex: []int64{0}}
var leaf0Minus2Request = trillian.GetLeavesByIndexRequest{LogId: logId1, LeafIndex: []int64{0, -2}}
var leaf03Request = trillian.GetLeavesByIndexRequest{LogId: logId1, LeafIndex: []int64{0, 3}}
var leaf0Log2Request = trillian.GetLeavesByIndexRequest{LogId: logId2, LeafIndex: []int64{0}}

var leaf1 = trillian.LogLeaf{SequenceNumber: 1, Leaf: trillian.Leaf{LeafHash: []byte("hash"), LeafValue: []byte("value"), ExtraData: []byte("extra")}}
var leaf3 = trillian.LogLeaf{SequenceNumber: 3, Leaf: trillian.Leaf{LeafHash: []byte("hash3"), LeafValue: []byte("value3"), ExtraData: []byte("extra3")}}
var expectedLeaf1 = trillian.LeafProto{LeafIndex: 1, LeafHash: []byte("hash"), LeafData: []byte("value"), ExtraData: []byte("extra")}
var expectedLeaf3 = trillian.LeafProto{LeafIndex: 3, LeafHash: []byte("hash3"), LeafData: []byte("value3"), ExtraData: []byte("extra3")}

var queueRequest0 = trillian.QueueLeavesRequest{LogId: logId1, Leaves: []*trillian.LeafProto{&expectedLeaf1}}
var queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logId2, Leaves: []*trillian.LeafProto{&expectedLeaf1}}
var queueRequestEmpty = trillian.QueueLeavesRequest{LogId: logId1, Leaves: []*trillian.LeafProto{}}

var getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logId1}
var getLogRootRequest2 = trillian.GetLatestSignedLogRootRequest{LogId: logId2}
var signedRoot1 = trillian.SignedLogRoot{TimestampNanos: 987654321, RootHash: []byte("A NICE HASH"), TreeSize: 7}

var getByHashRequest1 = trillian.GetLeavesByHashRequest{LogId: logId1, LeafHash: [][]byte{[]byte("test"), []byte("data")}}
var getByHashRequestBadHash = trillian.GetLeavesByHashRequest{LogId: logId1, LeafHash: [][]byte{[]byte(""), []byte("data")}}
var getByHashRequest2 = trillian.GetLeavesByHashRequest{LogId: logId2, LeafHash: [][]byte{[]byte("test"), []byte("data")}}

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Minus2Request)

	if err != nil || resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatalf("Returned non app level error response for negative leaf index")
	}
}

func TestGetLeavesByIndexBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}
}

func TestGetLeavesByIndexStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex",
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Request)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetLeavesByIndexInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Log2Request)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByIndexCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex",
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
		},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Request)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v")
	}

	if len(resp.Leaves) != 1 || !proto.Equal(resp.Leaves[0], &expectedLeaf1) {
		t.Fatalf("Expected leaf: %v but got: %v", expectedLeaf1, resp.Leaves[0])
	}
}

func TestGetLeavesByIndexMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{0, 3}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf03Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
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
}

func TestQueueLeavesStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves",
		func(t *storage.MockLogTX) {
			t.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}).Return(errors.New("STORAGE"))
		},
		func(s *TrillianLogServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestQueueLeavesInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0Log2)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestQueueLeavesCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves",
		func(t *storage.MockLogTX) {
			t.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}).Return(nil)
		},
		func(s *TrillianLogServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestQueueLeaves(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}).Return(nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.QueueLeaves(context.Background(), &queueRequest0)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v")
	}
}

func TestQueueLeavesNoLeavesRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.QueueLeaves(context.Background(), &queueRequestEmpty)

	if err != nil || resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatalf("Allowed zero leaves to be queued")
	}
}

func TestQueueLeavesBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeBeginFailsTest(t)
}

func TestGetLatestSignedLogRootBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}
}

func TestGetLatestSignedLogRootStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot",
		func(t *storage.MockLogTX) {
			t.EXPECT().LatestSignedLogRoot().Return(trillian.SignedLogRoot{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetLatestSignedLogRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot",
		func(t *storage.MockLogTX) { t.EXPECT().LatestSignedLogRoot().Return(trillian.SignedLogRoot{}, nil) },
		func(s *TrillianLogServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLatestSignedLogRootInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest2)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestGetLatestSignedLogRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().LatestSignedLogRoot().Return(signedRoot1, nil)
	mockTx.EXPECT().Commit().Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err != nil {
		t.Fatalf("Failed to get log root: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}

	if !proto.Equal(&signedRoot1, resp.SignedLogRoot) {
		t.Fatalf("Log root proto mismatch:\n%v\n%v", signedRoot1, resp.SignedLogRoot)
	}
}

func TestGetLeavesByHashInvalidHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	// This request includes an empty hash, which isn't allowed
	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequestBadHash)

	// Should have succeeded at RPC level
	if err != nil {
		t.Fatalf("Request failed with unexpected error: %v", err)
	}

	// And failed at app level
	if expected, got := trillian.TrillianApiStatusCode_ERROR, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level error status but got: %v", resp.Status.StatusCode)
	}
}

func TestGetLeavesByHashBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	_, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}
}

func TestGetLeavesByHashStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash",
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByHash([]trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestLeavesByHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash",
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByHash([]trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{}, nil)
		},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByHashInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash",
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest2)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByHash([]trillian.Hash{[]byte("test"), []byte("data")}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err != nil {
		t.Fatalf("Got error trying to get leaves by hash: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}

	if len(resp.Leaves) != 2 || !proto.Equal(resp.Leaves[0], &expectedLeaf1) || !proto.Equal(resp.Leaves[1], &expectedLeaf3) {
		t.Fatalf("Expected leaves %v and %v but got: %v", expectedLeaf1, expectedLeaf3, resp.Leaves)
	}
}

type prepareMockTXFunc func(*storage.MockLogTX)
type makeRpcFunc func(*TrillianLogServer) error

type parameterizedTest struct {
	ctrl      *gomock.Controller
	operation string
	prepareTx prepareMockTXFunc
	makeRpc   makeRpcFunc
}

func newParameterizedTest(ctrl *gomock.Controller, operation string, prepareTx prepareMockTXFunc, makeRpc makeRpcFunc) *parameterizedTest {
	return &parameterizedTest{ctrl, operation, prepareTx, makeRpc}
}

func (p *parameterizedTest) executeCommitFailsTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	p.prepareTx(mockTx)
	mockTx.EXPECT().Commit().Return(errors.New("Bang!"))
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	err := p.makeRpc(server)

	if err == nil {
		t.Fatalf("Returned OK when commit failed: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeInvalidLogIDTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	// Make a request for a nonexistent log id
	err := p.makeRpc(server)

	if err == nil || !strings.Contains(err.Error(), "BADLOGID") {
		t.Fatalf("Returned wrong error response for nonexistent log: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeStorageFailureTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	p.prepareTx(mockTx)
	mockTx.EXPECT().Rollback().Return(nil)

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	err := p.makeRpc(server)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong error response when storage failed: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeBeginFailsTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))

	server := NewTrillianLogServer(mockStorageProviderfunc(mockStorage))

	err := p.makeRpc(server)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed")
	}
}

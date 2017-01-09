package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"golang.org/x/net/context"
)

var th = merkle.NewRFC6962TreeHasher(crypto.NewSHA256())

var logID1 = int64(1)
var logID2 = int64(2)
var leaf0Request = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0}}
var leaf0Minus2Request = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, -2}}
var leaf03Request = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, 3}}
var leaf0Log2Request = trillian.GetLeavesByIndexRequest{LogId: logID2, LeafIndex: []int64{0}}

var leaf1Data = []byte("value")
var leaf3Data = []byte("value3")

var leaf1 = trillian.LogLeaf{LeafIndex: 1, MerkleLeafHash: th.HashLeaf(leaf1Data), LeafValue: leaf1Data, ExtraData: []byte("extra")}
var leaf3 = trillian.LogLeaf{LeafIndex: 3, MerkleLeafHash: th.HashLeaf(leaf3Data), LeafValue: leaf3Data, ExtraData: []byte("extra3")}

var queueRequest0 = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{&leaf1}}
var queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logID2, Leaves: []*trillian.LogLeaf{&leaf1}}
var queueRequestEmpty = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{}}

var getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logID1}
var getLogRootRequest2 = trillian.GetLatestSignedLogRootRequest{LogId: logID2}
var signedRoot1 = trillian.SignedLogRoot{TimestampNanos: 987654321, RootHash: []byte("A NICE HASH"), TreeSize: 7}

var getByHashRequest1 = trillian.GetLeavesByHashRequest{LogId: logID1, LeafHash: [][]byte{[]byte("test"), []byte("data")}}
var getByHashRequestBadHash = trillian.GetLeavesByHashRequest{LogId: logID1, LeafHash: [][]byte{[]byte(""), []byte("data")}}
var getByHashRequest2 = trillian.GetLeavesByHashRequest{LogId: logID2, LeafHash: [][]byte{[]byte("test"), []byte("data")}}

var getInclusionProofByHashRequestBadTreeSize = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: -50, LeafHash: []byte("data")}
var getInclusionProofByHashRequestBadHash = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 50, LeafHash: []byte{}}
var getInclusionProofByHashRequestRehash = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 25, LeafHash: []byte("ahash"), AllowRehashing: true}
var getInclusionProofByHashRequest7 = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 7, LeafHash: []byte("ahash")}
var getInclusionProofByHashRequest25 = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 25, LeafHash: []byte("ahash")}

var getInclusionProofByIndexRequestBadTreeSize = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: -50, LeafIndex: 10}
var getInclusionProofByIndexRequestBadLeafIndex = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: -10}
var getInclusionProofByIndexRequestBadLeafIndexRange = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: 60}
var getInclusionProofByIndexRequestRehash = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2, AllowRehashing: true}
var getInclusionProofByIndexRequest7 = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}
var getInclusionProofByIndexRequest25 = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: 25}

var getEntryAndProofRequestBadTreeSize = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: -20, LeafIndex: 20}
var getEntryAndProofRequestBadLeafIndex = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 25, LeafIndex: -5}
var getEntryAndProofRequestBadLeafIndexRange = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 25, LeafIndex: 30}
var getEntryAndProofRequestRehash = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 3, AllowRehashing: true}
var getEntryAndProofRequest17 = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 3}
var getEntryAndProofRequest7 = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}

var getConsistencyProofRequestBadFirstTreeSize = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: -10, SecondTreeSize: 25}
var getConsistencyProofRequestBadSecondTreeSize = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 10, SecondTreeSize: -25}
var getConsistencyProofRequestBadRange = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 330, SecondTreeSize: 329}
var getConsistencyProofRequestRehash = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 10, SecondTreeSize: 25, AllowRehashing: true}
var getConsistencyProofRequest25 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 10, SecondTreeSize: 25}
var getConsistencyProofRequest7 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 7}

var nodeIdsInclusionSize7Index2 = []storage.NodeID{
	testonly.MustCreateNodeIDForTreeCoords(0, 3, 64),
	testonly.MustCreateNodeIDForTreeCoords(1, 0, 64),
	testonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}

// Only one of the request fields should be set, depending on the request type being tested
type proofReqErrorTest struct {
	iReq        *trillian.GetInclusionProofRequest
	hReq        *trillian.GetInclusionProofByHashRequest
	pReq        *trillian.GetEntryAndProofRequest
	cReq        *trillian.GetConsistencyProofRequest
	expectedErr string // Expected string to be included in the error message
	msg         string // A message included in the test output if the test fails
}

var iProofReqErrorTests = []proofReqErrorTest{
	{iReq: &getInclusionProofByIndexRequestBadTreeSize, msg: "bad tree size", expectedErr: "tree size"},
	{iReq: &getInclusionProofByIndexRequestBadLeafIndex, msg: "bad leaf index", expectedErr: "leaf index"},
	{iReq: &getInclusionProofByIndexRequestBadLeafIndexRange, msg: "bad leaf index range", expectedErr: "does not exist"},
	{iReq: &getInclusionProofByIndexRequestRehash, msg: "rehashing requested", expectedErr: "rehash"},
}

var hProofReqErrorTests = []proofReqErrorTest{
	{hReq: &getInclusionProofByHashRequestBadTreeSize, msg: "bad tree size", expectedErr: "tree size"},
	{hReq: &getInclusionProofByHashRequestBadHash, msg: "bad hash", expectedErr: "invalid leaf hash"},
	{hReq: &getInclusionProofByHashRequestRehash, msg: "rehashing requested", expectedErr: "rehash"},
}

var pProofReqErrorTests = []proofReqErrorTest{
	{pReq: &getEntryAndProofRequestBadTreeSize, msg: "bad tree size", expectedErr: "tree size"},
	{pReq: &getEntryAndProofRequestBadLeafIndex, msg: "bad leaf index", expectedErr: "index:"},
	{pReq: &getEntryAndProofRequestBadLeafIndexRange, msg: "bad leaf index range", expectedErr: "exceeds tree size"},
	{pReq: &getEntryAndProofRequestRehash, msg: "rehashing requested", expectedErr: "rehash"},
}

var cProofReqErrorTests = []proofReqErrorTest{
	{cReq: &getConsistencyProofRequestBadFirstTreeSize, msg: "bad first size", expectedErr: "first tree size"},
	{cReq: &getConsistencyProofRequestBadSecondTreeSize, msg: "bad second size", expectedErr: "second tree size"},
	{cReq: &getConsistencyProofRequestBadRange, msg: "bad range", expectedErr: "must be > first"},
	{cReq: &getConsistencyProofRequestRehash, msg: "rehashing requested", expectedErr: "rehash"},
}

var nodeIdsConsistencySize4ToSize7 = []storage.NodeID{testonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}

func mockStorageProviderFunc(mockStorage storage.LogStorage) testonly.GetLogStorageFunc {
	return func(id int64) (storage.LogStorage, error) {
		if id == 1 {
			return mockStorage, nil
		}
		return nil, fmt.Errorf("BADLOGID: %d", id)
	}
}

func TestGetLeavesByIndexInvalidIndexRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Minus2Request)

	if err != nil || resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatalf("Returned non app level error response for negative leaf index: %d", resp.Status.StatusCode)
	}
}

func TestGetLeavesByIndexBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Snapshot().Return(mockTx, errors.New("TX"))
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

func TestGetLeavesByIndexStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Request)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetLeavesByIndexInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Log2Request)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByIndexCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
		},
		func(s *TrillianLogRPCServer) error {
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

	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{0}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}

	if len(resp.Leaves) != 1 || !proto.Equal(resp.Leaves[0], &leaf1) {
		t.Fatalf("Expected leaf: %v but got: %v", &leaf1, resp.Leaves[0])
	}
}

func TestGetLeavesByIndexMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{0, 3}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf03Request)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}

	if len(resp.Leaves) != 2 {
		t.Fatalf("Expected two leaves but got %d", len(resp.Leaves))
	}

	if !proto.Equal(resp.Leaves[0], &leaf1) {
		t.Fatalf("Expected leaf1: %v but got: %v", &leaf1, resp.Leaves[0])
	}

	if !proto.Equal(resp.Leaves[1], &leaf3) {
		t.Fatalf("Expected leaf3: %v but got: %v", &leaf3, resp.Leaves[0])
	}
}

func TestQueueLeavesStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTX) {
			t.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}, fakeTime).Return(errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestQueueLeavesInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0Log2)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestQueueLeavesCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTX) {
			t.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}, fakeTime).Return(nil)
		},
		func(s *TrillianLogRPCServer) error {
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
	mockTx.EXPECT().QueueLeaves([]trillian.LogLeaf{leaf1}, fakeTime).Return(nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.QueueLeaves(context.Background(), &queueRequest0)

	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}
}

func TestQueueLeavesNoLeavesRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.QueueLeaves(context.Background(), &queueRequestEmpty)

	if err != nil || resp.Status.StatusCode != trillian.TrillianApiStatusCode_ERROR {
		t.Fatal("Allowed zero leaves to be queued")
	}
}

func TestQueueLeavesBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
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

	mockStorage.EXPECT().Snapshot().Return(mockTx, errors.New("TX"))

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

func TestGetLatestSignedLogRootStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().LatestSignedLogRoot().Return(trillian.SignedLogRoot{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetLatestSignedLogRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot", readOnly,
		func(t *storage.MockLogTX) { t.EXPECT().LatestSignedLogRoot().Return(trillian.SignedLogRoot{}, nil) },
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLatestSignedLogRootInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
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

	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	mockTx.EXPECT().LatestSignedLogRoot().Return(signedRoot1, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

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
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

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

	mockStorage.EXPECT().Snapshot().Return(mockTx, errors.New("TX"))

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

func TestGetLeavesByHashStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestLeavesByHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByHashInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
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

	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)

	if err != nil {
		t.Fatalf("Got error trying to get leaves by hash: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, resp.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", resp.Status.StatusCode)
	}

	if len(resp.Leaves) != 2 || !proto.Equal(resp.Leaves[0], &leaf1) || !proto.Equal(resp.Leaves[1], &leaf3) {
		t.Fatalf("Expected leaves %v and %v but got: %v", &leaf1, &leaf3, resp.Leaves)
	}
}

func TestGetLeavesByLeafValueHashInvalidHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	// This request includes an empty hash, which isn't allowed
	resp, err := server.GetLeavesByLeafValueHash(context.Background(), &getByHashRequestBadHash)

	// Should have succeeded at RPC level
	if err != nil {
		t.Fatalf("GetLeavesByLeafValueHash(hash empty) = %v, %v, want: TrillianApiStatusCode_ERROR, nil", resp.Status, err)
	}

	// And failed at app level
	if got, want := resp.Status.StatusCode, trillian.TrillianApiStatusCode_ERROR; got != want {
		t.Fatalf("GetLeavesByLeafValueHash(hash empty) = %v, %v, want: %d, nil", resp.Status, err, want)
	}
}

func TestGetLeavesByLeafValueHashBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Snapshot().Return(mockTx, errors.New("TX"))

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetLeavesByLeafValueHash(context.Background(), &getByHashRequest1)
	if err == nil {
		t.Fatal("GetLeavesByLeafValueHash() = nil, want: error")
	}

	if !strings.Contains(err.Error(), "TX") {
		t.Fatalf("GetLeavesByLeafValueHash() = %v, want TX error", err)
	}
}

func TestGetLeavesByLeafValueHashStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByLeafValueHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByLeafValueHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByLeafValueHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestLeavesByLeafValueHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByLeafValueHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetLeavesByLeafValueHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByLeafValueHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetLeavesByLeafValueHashInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByLeafValueHash", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByLeafValueHash(context.Background(), &getByHashRequest2)
			return err
		})

	test.executeInvalidLogIDTest(t)
}

func TestGetLeavesByLeafValueHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByLeafValueHash([][]byte{[]byte("test"), []byte("data")}, false).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByLeafValueHash(context.Background(), &getByHashRequest1)

	if err != nil {
		t.Fatalf("GetLeavesByLeafValueHash = %v", err)
	}

	if got, want := resp.Status.StatusCode, trillian.TrillianApiStatusCode_OK; got != want {
		t.Fatalf("Bad status got: %v, want: %v", got, want)
	}

	if len(resp.Leaves) != 2 || !proto.Equal(resp.Leaves[0], &leaf1) || !proto.Equal(resp.Leaves[1], &leaf3) {
		t.Fatalf("Expected leaves %v and %v but got: %v", &leaf1, &leaf3, resp.Leaves)
	}
}

func TestGetProofByHashInvalidRequests(t *testing.T) {
	for _, test := range hProofReqErrorTests {
		ctrl := gomock.NewController(t)

		// Request should fail validation before any storage operations
		mockStorage := storage.NewMockLogStorage(ctrl)
		registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		_, err := server.GetInclusionProofByHash(context.Background(), test.hReq)

		if err == nil || !strings.Contains(err.Error(), test.expectedErr) {
			t.Errorf("want: error=...%s..., got: %v for test: %s, req: %v", test.expectedErr, err, test.msg, test.hReq)
		}

		ctrl.Finish()
	}
}

func TestGetProofByHashBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest25)
			return err
		})

	test.executeBeginFailsTest(t)
}

func TestGetProofByHashNoRevisionForTreeSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest25.TreeSize).Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest25)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetProofByHashNoLeafForHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest25.TreeSize).Return(int64(17), nil)
			t.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest25)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetProofByHashGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest7.TreeSize).Return(int64(3), nil)
			t.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{{LeafIndex: 2}}, nil)
			t.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetProofByHashWrongNodeCountFetched(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// The server expects three nodes from storage but we return only two
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected 3 nodes") {
		t.Fatalf("get inclusion proof by hash returned no or wrong error when get nodes returns wrong count: %v", err)
	}
}

func TestGetProofByHashWrongNodeReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// We set this up so one of the returned nodes has the wrong ID
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: testonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected node") || !strings.Contains(err.Error(), "at proof pos 1") {
		t.Fatalf("get inclusion proof by hash returned no or wrong error when get nodes returns wrong count: %v", err)
	}
}

func TestGetProofByHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
			t.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{{LeafIndex: 2}}, nil)
			t.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetProofByHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByHashRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetLeavesByHash([][]byte{[]byte("ahash")}, false).Return([]trillian.LogLeaf{{LeafIndex: 2}}, nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	proofResponse, err := server.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)

	if err != nil {
		t.Fatalf("get inclusion proof by hash should have succeeded but we got: %v", err)
	}

	if proofResponse == nil || proofResponse.Status == nil || proofResponse.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
		t.Fatalf("server response was not successful: %v", proofResponse)
	}

	nodeIDBytes1, err1 := proto.Marshal(nodeIdsInclusionSize7Index2[0].AsProto())
	nodeIDBytes2, err2 := proto.Marshal(nodeIdsInclusionSize7Index2[1].AsProto())
	nodeIDBytes3, err3 := proto.Marshal(nodeIdsInclusionSize7Index2[2].AsProto())

	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("failed to marshall test protos - should not happen: %v %v %v", err1, err2, err3)
	}

	expectedProof := trillian.Proof{LeafIndex: 2, ProofNode: []*trillian.Node{
		{NodeId: nodeIDBytes1, NodeHash: []byte("nodehash0"), NodeRevision: 3},
		{NodeId: nodeIDBytes2, NodeHash: []byte("nodehash1"), NodeRevision: 2},
		{NodeId: nodeIDBytes3, NodeHash: []byte("nodehash2"), NodeRevision: 3}}}

	if !proto.Equal(proofResponse.Proof[0], &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, proofResponse.Proof[0])
	}
}

func TestGetProofByIndexInvalidRequests(t *testing.T) {
	for _, test := range iProofReqErrorTests {
		ctrl := gomock.NewController(t)

		// Request should fail validation before any storage operations
		mockStorage := storage.NewMockLogStorage(ctrl)
		registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		_, err := server.GetInclusionProof(context.Background(), test.iReq)

		if err == nil || !strings.Contains(err.Error(), test.expectedErr) {
			t.Errorf("want: error=...%s..., got: %v for test: %s, req: %v", test.expectedErr, err, test.msg, test.iReq)
		}

		ctrl.Finish()
	}
}

func TestGetProofByIndexBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest25)
			return err
		})

	test.executeBeginFailsTest(t)
}

func TestGetProofByIndexNoRevisionForTreeSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest25.TreeSize).Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest25)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetProofByIndexGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
			t.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetProofByIndexWrongNodeCountFetched(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
	// The server expects three nodes from storage but we return only two
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected 3 nodes") {
		t.Fatalf("get inclusion proof by index returned no or wrong error when get nodes returns wrong count: %v", err)
	}
}

func TestGetProofByIndexWrongNodeReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
	// We set this up so one of the returned nodes has the wrong ID
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: testonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected node") || !strings.Contains(err.Error(), "at proof pos 1") {
		t.Fatalf("get inclusion proof by index returned no or wrong error when get nodes returns wrong count: %v", err)
	}
}

func TestGetProofByIndexCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
			t.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetProofByIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getInclusionProofByIndexRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	proofResponse, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)

	if err != nil {
		t.Fatalf("get inclusion proof by index should have succeeded but we got: %v", err)
	}

	if proofResponse == nil || proofResponse.Status == nil || proofResponse.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
		t.Fatalf("server response was not successful: %v", proofResponse)
	}

	nodeIDBytes1, err1 := proto.Marshal(nodeIdsInclusionSize7Index2[0].AsProto())
	nodeIDBytes2, err2 := proto.Marshal(nodeIdsInclusionSize7Index2[1].AsProto())
	nodeIDBytes3, err3 := proto.Marshal(nodeIdsInclusionSize7Index2[2].AsProto())

	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("failed to marshall test protos - should not happen: %v %v %v", err1, err2, err3)
	}

	expectedProof := trillian.Proof{LeafIndex: 2, ProofNode: []*trillian.Node{
		{NodeId: nodeIDBytes1, NodeHash: []byte("nodehash0"), NodeRevision: 3},
		{NodeId: nodeIDBytes2, NodeHash: []byte("nodehash1"), NodeRevision: 2},
		{NodeId: nodeIDBytes3, NodeHash: []byte("nodehash2"), NodeRevision: 3}}}

	if !proto.Equal(proofResponse.Proof, &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, proofResponse.Proof)
	}
}

func TestGetEntryAndProofInvalidRequests(t *testing.T) {
	for _, test := range pProofReqErrorTests {
		ctrl := gomock.NewController(t)

		// Request should fail validation before any storage operations
		mockStorage := storage.NewMockLogStorage(ctrl)
		registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		_, err := server.GetEntryAndProof(context.Background(), test.pReq)

		if err == nil || !strings.Contains(err.Error(), test.expectedErr) {
			t.Errorf("want: error=...%s..., got: %v for test: %s, req: %v", test.expectedErr, err, test.msg, test.pReq)
		}

		ctrl.Finish()
	}
}

func TestGetEntryAndProofBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)

	mockStorage.EXPECT().Snapshot().Return(nil, errors.New("BeginTX"))
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest17)

	if err == nil || !strings.Contains(err.Error(), "BeginTX") {
		t.Fatalf("get entry and proof returned no or wrong error when begin tx failed: %v", err)
	}
}

func TestGetEntryAndProofGetTreeSizeAtRevisionFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest17.TreeSize).Return(int64(0), errors.New("NOREVISION"))
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest17)

	if err == nil || !strings.Contains(err.Error(), "NOREVISION") {
		t.Fatalf("get entry and proof returned no or wrong error when no revision: %v", err)
	}
}

func TestGetEntryAndProofGetMerkleNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("GetNodes"))
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "GetNodes") {
		t.Fatalf("get entry and proof returned no or wrong error when get nodes failed: %v", err)
	}
}

func TestGetEntryAndProofGetLeavesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{2}).Return(nil, errors.New("GetLeaves"))
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "GetLeaves") {
		t.Fatalf("get entry and proof returned no or wrong error when get leaves failed: %v", err)
	}
}

func TestGetEntryAndProofGetLeavesReturnsMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	// Code passed one leaf index so expects one result, but we return more
	mockTx.EXPECT().GetLeavesByIndex([]int64{2}).Return([]trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected one leaf") {
		t.Fatalf("get entry and proof returned no or wrong error when storage returns multiple leaves: %v", err)
	}
}

func TestGetEntryAndProofCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{2}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("COMMIT"))

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "COMMIT") {
		t.Fatalf("get entry and proof returned no or wrong error when commit failed: %v", err)
	}
}

func TestGetEntryAndProof(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getEntryAndProofRequest7.TreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(3), nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex([]int64{2}).Return([]trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)

	if err != nil || response.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
		t.Fatalf("get entry and proof should have succeeded but we got: %v", err)
	}

	// Check the proof is the one we expected
	nodeIDBytes1, err1 := proto.Marshal(nodeIdsInclusionSize7Index2[0].AsProto())
	nodeIDBytes2, err2 := proto.Marshal(nodeIdsInclusionSize7Index2[1].AsProto())
	nodeIDBytes3, err3 := proto.Marshal(nodeIdsInclusionSize7Index2[2].AsProto())

	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("failed to marshall test protos - should not happen: %v %v %v", err1, err2, err3)
	}

	expectedProof := trillian.Proof{LeafIndex: 2, ProofNode: []*trillian.Node{
		{NodeId: nodeIDBytes1, NodeHash: []byte("nodehash0"), NodeRevision: 3},
		{NodeId: nodeIDBytes2, NodeHash: []byte("nodehash1"), NodeRevision: 2},
		{NodeId: nodeIDBytes3, NodeHash: []byte("nodehash2"), NodeRevision: 3}}}

	if !proto.Equal(response.Proof, &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, response.Proof)
	}

	// Check we got the correct leaf data
	if !proto.Equal(response.Leaf, &leaf1) {
		t.Fatalf("Expected leaf %v but got: %v", leaf1, response.Leaf)
	}
}

func TestGetSequencedLeafCountBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeBeginFailsTest(t)
}

func TestGetSequencedLeafCountStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetSequencedLeafCount().Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetSequencedLeafCountCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetSequencedLeafCount().Return(int64(27), nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetSequencedLeafCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetSequencedLeafCount().Return(int64(268), nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})

	if err != nil {
		t.Fatalf("expected no error getting leaf count but got: %v", err)
	}

	if response.Status == nil || response.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
		t.Fatalf("server response was not successful: %v", response)
	}

	if got, want := response.LeafCount, int64(268); got != want {
		t.Fatalf("expected leaf count: %d but got: %d", want, got)
	}
}

func TestGetConsistencyProofInvalidRequests(t *testing.T) {
	for _, test := range cProofReqErrorTests {
		ctrl := gomock.NewController(t)

		// Request should fail validation before any storage operations
		mockStorage := storage.NewMockLogStorage(ctrl)
		registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		_, err := server.GetConsistencyProof(context.Background(), test.cReq)

		if err == nil || !strings.Contains(err.Error(), test.expectedErr) {
			t.Errorf("want: error=...%s..., got: %v for test: %s, req: %v", test.expectedErr, err, test.msg, test.cReq)
		}

		ctrl.Finish()
	}
}

func TestGetConsistencyProofBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest25)
			return err
		})

	test.executeBeginFailsTest(t)
}

func TestGetConsistencyProofGetTreeRevision1Fails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest25.FirstTreeSize).Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest25)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetConsistencyProofGetTreeRevision2Fails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest25.FirstTreeSize).Return(int64(11), nil)
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest25.SecondTreeSize).Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest25)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetConsistencyProofGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.FirstTreeSize).Return(int64(3), nil)
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.SecondTreeSize).Return(int64(5), nil)
			t.EXPECT().GetMerkleNodes(int64(5), nodeIdsConsistencySize4ToSize7).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)
			return err
		})

	test.executeStorageFailureTest(t)
}

func TestGetConsistencyProofGetNodesReturnsWrongCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.FirstTreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.SecondTreeSize).Return(int64(5), nil)
	// The server expects one node from storage but we return two
	mockTx.EXPECT().GetMerkleNodes(int64(5), nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "expected 1 nodes") {
		t.Fatalf("get consistency proof returned no or wrong error when get nodes returns wrong count: %v", err)
	}
}

func TestGetConsistencyProofGetNodesReturnsWrongNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.FirstTreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.SecondTreeSize).Return(int64(5), nil)
	// Return an unexpected node that wasn't requested
	mockTx.EXPECT().GetMerkleNodes(int64(5), nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeID: testonly.MustCreateNodeIDForTreeCoords(1, 2, 64), NodeRevision: 3}}, nil)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)

	if err == nil || !strings.Contains(err.Error(), "at proof pos 0") {
		t.Fatalf("get consistency proof returned no or wrong error when get nodes returns wrong node: %v", err)
	}
}

func TestGetConsistencyProofCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTX) {
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.FirstTreeSize).Return(int64(3), nil)
			t.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.SecondTreeSize).Return(int64(5), nil)
			t.EXPECT().GetMerkleNodes(int64(5), nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeID: testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)
			return err
		})

	test.executeCommitFailsTest(t)
}

func TestGetConsistencyProof(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)
	mockStorage.EXPECT().Snapshot().Return(mockTx, nil)

	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.FirstTreeSize).Return(int64(3), nil)
	mockTx.EXPECT().GetTreeRevisionAtSize(getConsistencyProofRequest7.SecondTreeSize).Return(int64(5), nil)
	mockTx.EXPECT().GetMerkleNodes(int64(5), nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeID: testonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}}, nil)
	mockTx.EXPECT().Commit().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)

	if err != nil {
		t.Fatalf("failed to get consistency proof: %v", err)
	}

	if expected, got := trillian.TrillianApiStatusCode_OK, response.Status.StatusCode; expected != got {
		t.Fatalf("Expected app level ok status but got: %v", response.Status.StatusCode)
	}

	// Ensure we got the expected proof
	nodeIDBytes, err := proto.Marshal(nodeIdsConsistencySize4ToSize7[0].AsProto())

	if err != nil {
		t.Fatalf("failed to marshall test proto - should not happen: %v ", err)
	}

	expectedProof := trillian.Proof{LeafIndex: 0, ProofNode: []*trillian.Node{
		{NodeId: nodeIDBytes, NodeHash: []byte("nodehash"), NodeRevision: 3}}}

	if !proto.Equal(response.Proof, &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, response.Proof)
	}
}

type prepareMockTXFunc func(*storage.MockLogTX)
type makeRPCFunc func(*TrillianLogRPCServer) error

type txMode int

const (
	readOnly txMode = iota
	readWrite
)

type parameterizedTest struct {
	ctrl      *gomock.Controller
	operation string
	mode      txMode
	prepareTx prepareMockTXFunc
	makeRPC   makeRPCFunc
}

func newParameterizedTest(ctrl *gomock.Controller, operation string, m txMode, prepareTx prepareMockTXFunc, makeRPC makeRPCFunc) *parameterizedTest {
	return &parameterizedTest{ctrl, operation, m, prepareTx, makeRPC}
}

func (p *parameterizedTest) executeCommitFailsTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	switch p.mode {
	case readOnly:
		mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	case readWrite:
		mockStorage.EXPECT().Begin().Return(mockTx, nil)
	}
	p.prepareTx(mockTx)
	mockTx.EXPECT().Commit().Return(errors.New("bang"))
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	err := p.makeRPC(server)

	if err == nil {
		t.Fatalf("Returned OK when commit failed: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeInvalidLogIDTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	// Make a request for a nonexistent log id
	err := p.makeRPC(server)

	if err == nil || !strings.Contains(err.Error(), "BADLOGID") {
		t.Fatalf("Returned wrong error response for nonexistent log: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeStorageFailureTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	switch p.mode {
	case readOnly:
		mockStorage.EXPECT().Snapshot().Return(mockTx, nil)
	case readWrite:
		mockStorage.EXPECT().Begin().Return(mockTx, nil)
	}
	p.prepareTx(mockTx)
	mockTx.EXPECT().Rollback().Return(nil)

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	err := p.makeRPC(server)

	if err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong error response when storage failed: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeBeginFailsTest(t *testing.T) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTX(p.ctrl)

	switch p.mode {
	case readOnly:
		mockStorage.EXPECT().Snapshot().Return(mockTx, errors.New("TX"))
	case readWrite:
		mockStorage.EXPECT().Begin().Return(mockTx, errors.New("TX"))
	}

	registry := testonly.NewRegistryWithLogProvider(mockStorageProviderFunc(mockStorage))
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	err := p.makeRPC(server)

	if err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

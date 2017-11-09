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
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stestonly "github.com/google/trillian/storage/testonly"
)

var (
	th               = rfc6962.DefaultHasher
	logID1           = int64(1)
	logID2           = int64(2)
	leaf0Request     = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0}}
	leaf03Request    = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, 3}}
	leaf0Log2Request = trillian.GetLeavesByIndexRequest{LogId: logID2, LeafIndex: []int64{0}}
	leaf1Data        = []byte("value")
	leaf3Data        = []byte("value3")
	leaf1Hash, _     = th.HashLeaf(leaf1Data)
	leaf3Hash, _     = th.HashLeaf(leaf3Data)
	leaf1            = &trillian.LogLeaf{
		MerkleLeafHash: leaf1Hash,
		LeafValue:      leaf1Data,
		ExtraData:      []byte("extra"),
		LeafIndex:      1,
	}
	leaf3 = &trillian.LogLeaf{
		MerkleLeafHash: leaf3Hash,
		LeafValue:      leaf3Data,
		ExtraData:      []byte("extra3"),
		LeafIndex:      3,
	}

	queueRequest0     = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{leaf1}}
	queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logID2, Leaves: []*trillian.LogLeaf{leaf1}}

	getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logID1}
	revision1          = int64(5)
	signedRoot1        = trillian.SignedLogRoot{TimestampNanos: 987654321, RootHash: []byte("A NICE HASH"), TreeSize: 7, TreeRevision: revision1}

	getByHashRequest1 = trillian.GetLeavesByHashRequest{LogId: logID1, LeafHash: [][]byte{[]byte("test"), []byte("data")}}
	getByHashRequest2 = trillian.GetLeavesByHashRequest{LogId: logID2, LeafHash: [][]byte{[]byte("test"), []byte("data")}}

	getInclusionProofByHashRequest7  = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 7, LeafHash: []byte("ahash")}
	getInclusionProofByHashRequest25 = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 25, LeafHash: []byte("ahash")}

	getInclusionProofByIndexRequest7  = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}
	getInclusionProofByIndexRequest25 = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: 25}

	getEntryAndProofRequest17 = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 3}
	getEntryAndProofRequest7  = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}

	getConsistencyProofRequest7  = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 7}
	getConsistencyProofRequest44 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 4}
	getConsistencyProofRequest48 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 8}

	nodeIdsInclusionSize7Index2 = []storage.NodeID{
		stestonly.MustCreateNodeIDForTreeCoords(0, 3, 64),
		stestonly.MustCreateNodeIDForTreeCoords(1, 0, 64),
		stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}

	nodeIdsConsistencySize4ToSize7 = []storage.NodeID{stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}
)

func TestGetLeavesByIndexBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), leaf0Request.LogId).Return(nil, errors.New("TX"))
	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, leaf0Request.LogId),
		LogStorage:   mockStorage,
	}
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
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return(nil, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Request)
			return err
		})

	test.executeStorageFailureTest(t, leaf0Request.LogId)
}

func TestGetLeavesByIndexInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Log2Request)
			return err
		})

	test.executeInvalidLogIDTest(t, true /* snapshot */)
}

func TestGetLeavesByIndexCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByIndex(context.Background(), &leaf0Request)
			return err
		})

	test.executeCommitFailsTest(t, leaf0Request.LogId)
}

func TestGetLeavesByIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), leaf0Request.LogId).Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, leaf0Request.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf0Request)
	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if len(resp.Leaves) != 1 || !proto.Equal(resp.Leaves[0], leaf1) {
		t.Fatalf("Expected leaf: %v but got: %v", leaf1, resp.Leaves[0])
	}
}

func TestGetLeavesByIndexMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), leaf03Request.LogId).Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0, 3}).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, leaf03Request.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByIndex(context.Background(), &leaf03Request)
	if err != nil {
		t.Fatalf("Failed to get leaf by index: %v", err)
	}

	if len(resp.Leaves) != 2 {
		t.Fatalf("Expected two leaves but got %d", len(resp.Leaves))
	}

	if !proto.Equal(resp.Leaves[0], leaf1) {
		t.Fatalf("Expected leaf1: %v but got: %v", leaf1, resp.Leaves[0])
	}

	if !proto.Equal(resp.Leaves[1], leaf3) {
		t.Fatalf("Expected leaf3: %v but got: %v", leaf3, resp.Leaves[0])
	}
}

func TestQueueLeavesStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().QueueLeaves(gomock.Any(), []*trillian.LogLeaf{leaf1}, fakeTime).Return(nil, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeStorageFailureTest(t, queueRequest0.LogId)
}

func TestQueueLeavesInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0Log2)
			return err
		})

	test.executeInvalidLogIDTest(t, false /* snapshot */)
}

func TestQueueLeavesCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().QueueLeaves(gomock.Any(), []*trillian.LogLeaf{leaf1}, fakeTime).Return([]*trillian.LogLeaf{nil}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeCommitFailsTest(t, queueRequest0.LogId)
}

func TestQueueLeaves(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().BeginForTree(gomock.Any(), queueRequest0.LogId).Return(mockTx, nil)
	mockTx.EXPECT().QueueLeaves(gomock.Any(), []*trillian.LogLeaf{leaf1}, fakeTime).Return([]*trillian.LogLeaf{nil}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, queueRequest0.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	rsp, err := server.QueueLeaves(ctx, &queueRequest0)
	if err != nil {
		t.Fatalf("Failed to queue leaf: %v", err)
	}
	if len(rsp.QueuedLeaves) != 1 {
		t.Errorf("QueueLeaves() returns %d leaves; want 1", len(rsp.QueuedLeaves))
	}
	queuedLeaf := rsp.QueuedLeaves[0]
	if queuedLeaf.Status != nil && queuedLeaf.Status.Code != int32(code.Code_OK) {
		t.Errorf("QueueLeaves().Status=%d,nil; want %d,nil", queuedLeaf.Status.Code, code.Code_OK)
	}
	if !proto.Equal(queueRequest0.Leaves[0], queuedLeaf.Leaf) {
		diff := pretty.Compare(queueRequest0.Leaves[0], queuedLeaf.Leaf)
		t.Errorf("post-QueueLeaves() diff:\n%v", diff)
	}

	// Repeating the operation gives ALREADY_EXISTS.
	server.registry.AdminStorage = mockAdminStorage(ctrl, queueRequest0.LogId)
	mockStorage.EXPECT().BeginForTree(gomock.Any(), queueRequest0.LogId).Return(mockTx, nil)
	mockTx.EXPECT().QueueLeaves(gomock.Any(), []*trillian.LogLeaf{leaf1}, fakeTime).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	rsp, err = server.QueueLeaves(ctx, &queueRequest0)
	if err != nil {
		t.Fatalf("Failed to re-queue leaf: %v", err)
	}
	if len(rsp.QueuedLeaves) != 1 {
		t.Errorf("QueueLeaves() returns %d leaves; want 1", len(rsp.QueuedLeaves))
	}
	queuedLeaf = rsp.QueuedLeaves[0]
	if queuedLeaf.Status == nil || queuedLeaf.Status.Code != int32(code.Code_ALREADY_EXISTS) {
		t.Errorf("QueueLeaves().Status=%d,nil; want %d,nil", queuedLeaf.Status.Code, code.Code_ALREADY_EXISTS)
	}
	if !proto.Equal(queueRequest0.Leaves[0], queuedLeaf.Leaf) {
		diff := pretty.Compare(queueRequest0.Leaves[0], queuedLeaf.Leaf)
		t.Errorf("post-QueueLeaves() diff:\n%v", diff)
	}
}

func TestQueueLeavesBeginFailsCausesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", readWrite,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeBeginFailsTest(t, queueRequest0.LogId)
}

type latestRootTest struct {
	req         trillian.GetLatestSignedLogRootRequest
	wantRoot    trillian.GetLatestSignedLogRootResponse
	errStr      string
	noSnap      bool
	snapErr     error
	noRoot      bool
	storageRoot trillian.SignedLogRoot
	rootErr     error
	noCommit    bool
	commitErr   error
	noClose     bool
}

func TestGetLatestSignedLogRoot2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []latestRootTest{
		{
			// Test error case when failing to get a snapshot from storage.
			req:      getLogRootRequest1,
			snapErr:  errors.New("SnapshotForTree() error"),
			errStr:   "SnapshotFor",
			noRoot:   true,
			noCommit: true,
			noClose:  true,
		},
		{
			// Test error case when storage fails to provide a root.
			req:      getLogRootRequest1,
			errStr:   "LatestSigned",
			rootErr:  errors.New("LatestSignedLogRoot() error"),
			noCommit: true,
		},
		{
			// Test error case where storage fails to commit the tx.
			req:       getLogRootRequest1,
			errStr:    "commit",
			commitErr: errors.New("commit() error"),
		},
		{
			// Test normal case where a root is returned correctly.
			req:         getLogRootRequest1,
			wantRoot:    trillian.GetLatestSignedLogRootResponse{SignedLogRoot: &signedRoot1},
			storageRoot: signedRoot1,
		},
	}

	for _, test := range tests {
		mockStorage := storage.NewMockLogStorage(ctrl)
		mockTx := storage.NewMockLogTreeTX(ctrl)
		if !test.noSnap {
			mockStorage.EXPECT().SnapshotForTree(gomock.Any(), test.req.LogId).Return(mockTx, test.snapErr)
		}
		if !test.noRoot {
			mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(test.storageRoot, test.rootErr)
		}
		if !test.noCommit {
			mockTx.EXPECT().Commit().Return(test.commitErr)
		}
		if !test.noClose {
			mockTx.EXPECT().Close().Return(nil)
		}

		registry := extension.Registry{
			AdminStorage: mockAdminStorage(ctrl, test.req.LogId),
			LogStorage:   mockStorage,
		}
		s := NewTrillianLogRPCServer(registry, fakeTimeSource)
		got, err := s.GetLatestSignedLogRoot(context.Background(), &test.req)
		if len(test.errStr) > 0 {
			if err == nil || !strings.Contains(err.Error(), test.errStr) {
				t.Errorf("GetLatestSignedLogRoot(%+v)=_,nil, want: _,err contains: %s but got: %v", test.req, test.errStr, err)
			}
		} else {
			if err != nil {
				t.Errorf("GetLatestSignedLogRoot(%+v)=_,%v, want: _,nil", test.req, err)
				continue
			}
			// Ensure we got the expected root back.
			if !proto.Equal(got.SignedLogRoot, test.wantRoot.SignedLogRoot) {
				t.Errorf("GetConsistencyProof(%+v)=%v,nil, want: %v,nil", test.req, got, test.wantRoot)
			}
		}
	}
}

func TestGetLeavesByHashBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getByHashRequest1.LogId).Return(nil, errors.New("TX"))
	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getByHashRequest1.LogId),
		LogStorage:   mockStorage,
	}
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
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("test"), []byte("data")}, false).Return(nil, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeStorageFailureTest(t, getByHashRequest1.LogId)
}

func TestLeavesByHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("test"), []byte("data")}, false).Return(nil, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest1)
			return err
		})

	test.executeCommitFailsTest(t, getByHashRequest1.LogId)
}

func TestGetLeavesByHashInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLeavesByHash(context.Background(), &getByHashRequest2)
			return err
		})

	test.executeInvalidLogIDTest(t, true /* snapshot */)
}

func TestGetLeavesByHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getByHashRequest1.LogId).Return(mockTx, nil)
	mockTx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("test"), []byte("data")}, false).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getByHashRequest1.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLeavesByHash(context.Background(), &getByHashRequest1)
	if err != nil {
		t.Fatalf("Got error trying to get leaves by hash: %v", err)
	}

	if len(resp.Leaves) != 2 || !proto.Equal(resp.Leaves[0], leaf1) || !proto.Equal(resp.Leaves[1], leaf3) {
		t.Fatalf("Expected leaves %v and %v but got: %v", leaf1, leaf3, resp.Leaves)
	}
}

func TestGetProofByHashBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest25)
			return err
		})

	test.executeBeginFailsTest(t, getInclusionProofByHashRequest25.LogId)
}

func TestGetProofByHashNoLeafForHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return(nil, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest25)
			return err
		})

	test.executeStorageFailureTest(t, getInclusionProofByHashRequest25.LogId)
}

func TestGetProofByHashGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
			return err
		})

	test.executeStorageFailureTest(t, getInclusionProofByHashRequest7.LogId)
}

func TestGetProofByHashWrongNodeCountFetched(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByHashRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// The server expects three nodes from storage but we return only two
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByHashRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByHashRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// We set this up so one of the returned nodes has the wrong ID
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: stestonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByHashRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
	if err == nil || !strings.Contains(err.Error(), "expected node ") {
		t.Fatalf("get inclusion proof by hash returned no or wrong error when get nodes returns wrong node: %v", err)
	}
}

func TestGetProofByHashCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
			return err
		})

	test.executeCommitFailsTest(t, getInclusionProofByHashRequest7.LogId)
}

func TestGetProofByHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByHashRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByHashRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	proofResponse, err := server.GetInclusionProofByHash(context.Background(), &getInclusionProofByHashRequest7)
	if err != nil {
		t.Fatalf("get inclusion proof by hash should have succeeded but we got: %v", err)
	}

	if proofResponse == nil {
		t.Fatalf("server response was not successful: %v", proofResponse)
	}

	expectedProof := trillian.Proof{
		LeafIndex: 2,
		Hashes: [][]byte{
			[]byte("nodehash0"),
			[]byte("nodehash1"),
			[]byte("nodehash2"),
		},
	}

	if !proto.Equal(proofResponse.Proof[0], &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, proofResponse.Proof[0])
	}
}

func TestGetProofByIndexBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest25)
			return err
		})

	test.executeBeginFailsTest(t, getInclusionProofByIndexRequest25.LogId)
}

func TestGetProofByIndexGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
			return err
		})

	test.executeStorageFailureTest(t, getInclusionProofByIndexRequest7.LogId)
}

func TestGetProofByIndexWrongNodeCountFetched(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByHashRequest7.LogId).Return(mockTx, nil)

	// The server expects three nodes from storage but we return only two
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByHashRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByIndexRequest7.LogId).Return(mockTx, nil)

	// We set this up so one of the returned nodes has the wrong ID
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: stestonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByIndexRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
	if err == nil || !strings.Contains(err.Error(), "expected node ") {
		t.Fatalf("get inclusion proof by index returned no or wrong error when get nodes returns wrong node: %v", err)
	}
}

func TestGetProofByIndexCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
			return err
		})

	test.executeCommitFailsTest(t, getInclusionProofByIndexRequest7.LogId)
}

func TestGetProofByIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getInclusionProofByIndexRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getInclusionProofByIndexRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	proofResponse, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest7)
	if err != nil {
		t.Fatalf("get inclusion proof by index should have succeeded but we got: %v", err)
	}

	if proofResponse == nil {
		t.Fatalf("server response was not successful: %v", proofResponse)
	}

	expectedProof := trillian.Proof{
		LeafIndex: 2,
		Hashes: [][]byte{
			[]byte("nodehash0"),
			[]byte("nodehash1"),
			[]byte("nodehash2"),
		},
	}

	if !proto.Equal(proofResponse.Proof, &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, proofResponse.Proof)
	}
}

func TestGetEntryAndProofBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest17.LogId).Return(nil, errors.New("BeginTX"))
	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest17.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest17)
	if err == nil || !strings.Contains(err.Error(), "BeginTX") {
		t.Fatalf("get entry and proof returned no or wrong error when begin tx failed: %v", err)
	}
}

func TestGetEntryAndProofGetMerkleNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("GetNodes"))
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return(nil, errors.New("GetLeaves"))
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	// Code passed one leaf index so expects one result, but we return more
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(errors.New("COMMIT"))
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getEntryAndProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getEntryAndProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest7)
	if err != nil {
		t.Fatalf("get entry and proof should have succeeded but we got: %v", err)
	}

	// Check the proof is the one we expected
	expectedProof := trillian.Proof{
		LeafIndex: 2,
		Hashes: [][]byte{
			[]byte("nodehash0"),
			[]byte("nodehash1"),
			[]byte("nodehash2"),
		},
	}

	if !proto.Equal(response.Proof, &expectedProof) {
		t.Fatalf("expected proof: %v but got: %v", expectedProof, response.Proof)
	}

	// Check we got the correct leaf data
	if !proto.Equal(response.Leaf, leaf1) {
		t.Fatalf("Expected leaf %v but got: %v", leaf1, response.Leaf)
	}
}

func TestGetSequencedLeafCountBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeBeginFailsTest(t, logID1)
}

func TestGetSequencedLeafCountStorageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetSequencedLeafCount(gomock.Any()).Return(int64(0), errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeStorageFailureTest(t, logID1)
}

func TestGetSequencedLeafCountCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetSequencedLeafCount(gomock.Any()).Return(int64(27), nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
			return err
		})

	test.executeCommitFailsTest(t, logID1)
}

func TestGetSequencedLeafCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), logID1).Return(mockTx, nil)

	mockTx.EXPECT().GetSequencedLeafCount(gomock.Any()).Return(int64(268), nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, logID1),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetSequencedLeafCount(context.Background(), &trillian.GetSequencedLeafCountRequest{LogId: logID1})
	if err != nil {
		t.Fatalf("expected no error getting leaf count but got: %v", err)
	}

	if got, want := response.LeafCount, int64(268); got != want {
		t.Fatalf("expected leaf count: %d but got: %d", want, got)
	}
}

type consistProofTest struct {
	req         trillian.GetConsistencyProofRequest
	errStr      string
	wantHashes  [][]byte
	noSnap      bool
	snapErr     error
	noRoot      bool
	rootErr     error
	noRev       bool
	nodeIDs     []storage.NodeID
	nodes       []storage.Node
	getNodesErr error
	noCommit    bool
	commitErr   error
	noClose     bool
}

func TestGetConsistencyProof(t *testing.T) {
	tests := []consistProofTest{
		{
			// Storage snapshot fails, should result in an error. Happens before we have a TX so
			// no Close() etc.
			req:      getConsistencyProofRequest7,
			errStr:   "SnapshotFor",
			snapErr:  errors.New("SnapshotForTree() failed"),
			noRoot:   true,
			noRev:    true,
			noCommit: true,
			noClose:  true,
		},
		{
			// Storage fails to read the log root, should result in an error.
			req:      getConsistencyProofRequest7,
			errStr:   "LatestSigned",
			rootErr:  errors.New("LatestSignedLogRoot() failed"),
			noRev:    true,
			noCommit: true,
		},
		{
			// Storage fails to get nodes, should result in an error
			req:         getConsistencyProofRequest7,
			errStr:      "getMerkle",
			nodeIDs:     nodeIdsConsistencySize4ToSize7,
			wantHashes:  [][]byte{[]byte("nodehash")},
			nodes:       []storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}},
			getNodesErr: errors.New("getMerkleNodes() failed"),
			noCommit:    true,
		},
		{
			// Storage fails to commit, should result in an error.
			req:        getConsistencyProofRequest7,
			errStr:     "commit",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}},
			commitErr:  errors.New("commit() failed"),
		},
		{
			// Storage doesn't return the requested node, should result in an error.
			req:        getConsistencyProofRequest7,
			errStr:     "expected node {{[0 0 0 0 0 0 0 4] 62}",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(3, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}},
			noCommit:   true,
		},
		{
			// Storage returns an unexpected extra node, should result in an error.
			req:        getConsistencyProofRequest7,
			errStr:     "expected 1 nodes",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}, {NodeID: stestonly.MustCreateNodeIDForTreeCoords(3, 10, 64), NodeRevision: 37, Hash: []byte("nodehash2")}},
			noCommit:   true,
		},
		{
			// Ask for a proof from size 4 to 8 but the tree is only size 7. This should fail.
			req:        getConsistencyProofRequest48,
			errStr:     "snapshot2 8 > treeSize 7",
			wantHashes: [][]byte{},
			nodeIDs:    nil,
			noRev:      true,
			noCommit:   true,
		},
		{
			// A normal request which should succeed.
			req:        getConsistencyProofRequest7,
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}},
		},
		{
			// Tests first==second edge case, which should succeed but is an empty proof.
			req:        getConsistencyProofRequest44,
			wantHashes: [][]byte{},
			nodeIDs:    []storage.NodeID{},
			nodes:      []storage.Node{},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, test := range tests {
		mockStorage := storage.NewMockLogStorage(ctrl)
		mockTx := storage.NewMockLogTreeTX(ctrl)
		if !test.noSnap {
			mockStorage.EXPECT().SnapshotForTree(gomock.Any(), test.req.LogId).Return(mockTx, test.snapErr)
		}
		if !test.noRoot {
			mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, test.rootErr)
		}
		if !test.noRev {
			mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
		}
		if test.nodeIDs != nil {
			mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, test.nodeIDs).Return(test.nodes, test.getNodesErr)
		}
		if !test.noCommit {
			mockTx.EXPECT().Commit().Return(test.commitErr)
		}
		if !test.noClose {
			mockTx.EXPECT().Close().Return(nil)
		}

		registry := extension.Registry{
			AdminStorage: mockAdminStorage(ctrl, test.req.LogId),
			LogStorage:   mockStorage,
		}
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)
		response, err := server.GetConsistencyProof(context.Background(), &test.req)

		if len(test.errStr) > 0 {
			if err == nil || !strings.Contains(err.Error(), test.errStr) {
				t.Errorf("GetConsistencyProof(%+v)=_, %v; want _, err containing %q", test.req, err, test.errStr)
			}
		} else {
			if err != nil {
				t.Errorf("GetConsistencyProof(%+v)=_,%v; want: _,nil", test.req, err)
				continue
			}
			// Ensure we got the expected proof.
			wantProof := trillian.Proof{
				LeafIndex: 0,
				Hashes:    test.wantHashes,
			}
			if got, want := response.Proof, &wantProof; !proto.Equal(got, want) {
				t.Errorf("GetConsistencyProof(%+v)=%v,nil, want: %v,nil", test.req, got, want)
			}
		}
	}
}

func TestTrillianLogRPCServer_GetConsistencyProofErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetConsistencyProofRequest
	}{
		{
			desc: "badFirstSize",
			req: &trillian.GetConsistencyProofRequest{
				LogId:          1,
				FirstTreeSize:  -10,
				SecondTreeSize: 20,
			},
		},
		{
			desc: "badSecondSize",
			req: &trillian.GetConsistencyProofRequest{
				LogId:          1,
				FirstTreeSize:  10,
				SecondTreeSize: -20,
			},
		},
		{
			desc: "firstGreaterThanSecond",
			req: &trillian.GetConsistencyProofRequest{
				LogId:          1,
				FirstTreeSize:  10,
				SecondTreeSize: 9,
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetConsistencyProof(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetConsistencyProof() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_GetEntryAndProofErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetEntryAndProofRequest
	}{
		{
			desc: "badLeafIndex",
			req: &trillian.GetEntryAndProofRequest{
				LogId:     1,
				LeafIndex: -10,
				TreeSize:  20,
			},
		},
		{
			desc: "badTreeSize",
			req: &trillian.GetEntryAndProofRequest{
				LogId:     1,
				LeafIndex: 10,
				TreeSize:  -20,
			},
		},
		{
			desc: "indexGreaterThanSize",
			req: &trillian.GetEntryAndProofRequest{
				LogId:     1,
				LeafIndex: 10,
				TreeSize:  9,
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetEntryAndProof(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetEntryAndProof() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_GetInclusionProofErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetInclusionProofRequest
	}{
		{
			desc: "badLeafIndex",
			req: &trillian.GetInclusionProofRequest{
				LogId:     1,
				LeafIndex: -10,
				TreeSize:  20,
			},
		},
		{
			desc: "badTreeSize",
			req: &trillian.GetInclusionProofRequest{
				LogId:     1,
				LeafIndex: 10,
				TreeSize:  -20,
			},
		},
		{
			desc: "indexGreaterThanSize",
			req: &trillian.GetInclusionProofRequest{
				LogId:     1,
				LeafIndex: 10,
				TreeSize:  9,
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetInclusionProof(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetInclusionProof() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_GetInclusionProofByHashErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetInclusionProofByHashRequest
	}{
		{
			desc: "nilLeafHash",
			req: &trillian.GetInclusionProofByHashRequest{
				LogId:    1,
				TreeSize: 20,
			},
		},
		{
			desc: "badTreeSize",
			req: &trillian.GetInclusionProofByHashRequest{
				LogId:    1,
				LeafHash: []byte("32.bytes.hash..................."),
				TreeSize: -20,
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetInclusionProofByHash(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetInclusionProofByHash() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_GetLeavesByHashErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetLeavesByHashRequest
	}{
		{
			desc: "nilLeafHashes",
			req: &trillian.GetLeavesByHashRequest{
				LogId: 1,
			},
		},
		{
			desc: "nilLeafHash",
			req: &trillian.GetLeavesByHashRequest{
				LogId: 1,
				LeafHash: [][]byte{
					[]byte("32.bytes.hash.a................."),
					nil,
					[]byte("32.bytes.hash.b................."),
				},
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetLeavesByHash(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetLeavesByHash() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_GetLeavesByIndexErrors(t *testing.T) {
	tests := []struct {
		desc string
		req  *trillian.GetLeavesByIndexRequest
	}{
		{
			desc: "nilLeafIndex",
			req: &trillian.GetLeavesByIndexRequest{
				LogId: 1,
			},
		},
		{
			desc: "badLeadIndex",
			req: &trillian.GetLeavesByIndexRequest{
				LogId:     1,
				LeafIndex: []int64{10, -11, 12},
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.GetLeavesByIndex(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: GetLeavesByIndex() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_QueueLeafErrors(t *testing.T) {
	leafValue := []byte("leaf value")

	tests := []struct {
		desc string
		req  *trillian.QueueLeafRequest
	}{
		{
			desc: "nilLeaf",
			req: &trillian.QueueLeafRequest{
				LogId: 1,
			},
		},
		{
			desc: "nilLeafValue",
			req: &trillian.QueueLeafRequest{
				LogId: 1,
				Leaf:  &trillian.LogLeaf{},
			},
		},
		{
			desc: "badLeafIndex",
			req: &trillian.QueueLeafRequest{
				LogId: 1,
				Leaf: &trillian.LogLeaf{
					LeafValue: leafValue,
					LeafIndex: -10,
				},
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.QueueLeaf(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: QueueLeaf() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestTrillianLogRPCServer_QueueLeavesErrors(t *testing.T) {
	leafValue := []byte("leaf value")
	goodLeaf := &trillian.LogLeaf{
		LeafValue: leafValue,
	}

	tests := []struct {
		desc string
		req  *trillian.QueueLeavesRequest
	}{
		{
			desc: "nilLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId: 1,
			},
		},
		{
			desc: "nilLeaf",
			req: &trillian.QueueLeavesRequest{
				LogId:  1,
				Leaves: []*trillian.LogLeaf{goodLeaf, nil},
			},
		},
		{
			desc: "nilLeafValue",
			req: &trillian.QueueLeavesRequest{
				LogId: 1,
				Leaves: []*trillian.LogLeaf{
					goodLeaf,
					{},
				},
			},
		},
		{
			desc: "badLeafIndex",
			req: &trillian.QueueLeavesRequest{
				LogId: 1,
				Leaves: []*trillian.LogLeaf{
					goodLeaf,
					{
						LeafValue: leafValue,
						LeafIndex: -10,
					},
				},
			},
		},
	}

	logServer := NewTrillianLogRPCServer(extension.Registry{}, fakeTimeSource)
	ctx := context.Background()
	for _, test := range tests {
		_, err := logServer.QueueLeaves(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
			t.Errorf("%v: QueueLeaves() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
		}
	}
}

type prepareMockTXFunc func(*storage.MockLogTreeTX)
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

func (p *parameterizedTest) executeCommitFailsTest(t *testing.T, logID int64) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTreeTX(p.ctrl)

	switch p.mode {
	case readOnly:
		mockStorage.EXPECT().SnapshotForTree(gomock.Any(), logID).Return(mockTx, nil)
	case readWrite:
		mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	}
	p.prepareTx(mockTx)
	mockTx.EXPECT().Commit().Return(errors.New("bang"))
	mockTx.EXPECT().Close().Return(errors.New("bang"))
	mockTx.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(p.ctrl, logID),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if err := p.makeRPC(server); err == nil {
		t.Fatalf("returned OK when commit failed: %s", p.operation)
	}
}

func (p *parameterizedTest) executeInvalidLogIDTest(t *testing.T, snapshot bool) {
	badLogErr := errors.New("BADLOGID")

	adminStorage := storage.NewMockAdminStorage(p.ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(p.ctrl)
	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(1).Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), gomock.Any()).MaxTimes(1).Return(nil, badLogErr)
	adminTX.EXPECT().Close().MaxTimes(1).Return(nil)

	mockStorage := storage.NewMockLogStorage(p.ctrl)
	if ctx, logID := gomock.Any(), int64(2); snapshot {
		mockStorage.EXPECT().SnapshotForTree(ctx, logID).MaxTimes(1).Return(nil, badLogErr)
	} else {
		mockStorage.EXPECT().BeginForTree(ctx, logID).MaxTimes(1).Return(nil, badLogErr)
	}

	registry := extension.Registry{
		AdminStorage: adminStorage,
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	// Make a request for a nonexistent log id
	if err := p.makeRPC(server); err == nil || !strings.Contains(err.Error(), badLogErr.Error()) {
		t.Fatalf("Returned wrong error response for nonexistent log: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeStorageFailureTest(t *testing.T, logID int64) {
	mockStorage := storage.NewMockLogStorage(p.ctrl)
	mockTx := storage.NewMockLogTreeTX(p.ctrl)

	switch p.mode {
	case readOnly:
		mockStorage.EXPECT().SnapshotForTree(gomock.Any(), logID).Return(mockTx, nil)
	case readWrite:
		mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	}
	p.prepareTx(mockTx)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(p.ctrl, logID),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if err := p.makeRPC(server); err == nil || !strings.Contains(err.Error(), "STORAGE") {
		t.Fatalf("Returned wrong error response when storage failed: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeBeginFailsTest(t *testing.T, logID int64) {
	logStorage := storage.NewMockLogStorage(p.ctrl)
	logTX := storage.NewMockLogTreeTX(p.ctrl)

	switch p.mode {
	case readOnly:
		logStorage.EXPECT().SnapshotForTree(gomock.Any(), logID).Return(logTX, errors.New("TX"))
	case readWrite:
		logStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(logTX, errors.New("TX"))
	}

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(p.ctrl, logID),
		LogStorage:   logStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if err := p.makeRPC(server); err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

func mockAdminStorage(ctrl *gomock.Controller, treeID int64) storage.AdminStorage {
	tree := *stestonly.LogTree
	tree.TreeId = treeID

	adminStorage := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)

	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(1).Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), treeID).MaxTimes(1).Return(&tree, nil)
	adminTX.EXPECT().Close().MaxTimes(1).Return(nil)
	adminTX.EXPECT().Commit().MaxTimes(1).Return(nil)

	return adminStorage
}

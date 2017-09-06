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
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/genproto/googleapis/rpc/code"
)

var (
	th                 = rfc6962.DefaultHasher
	logID1             = int64(1)
	logID2             = int64(2)
	leaf0Request       = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0}}
	leaf0Minus2Request = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, -2}}
	leaf03Request      = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, 3}}
	leaf0Log2Request   = trillian.GetLeavesByIndexRequest{LogId: logID2, LeafIndex: []int64{0}}
	leaf1Data          = []byte("value")
	leaf3Data          = []byte("value3")
	leaf1              = &trillian.LogLeaf{LeafIndex: 1, MerkleLeafHash: th.HashLeaf(leaf1Data), LeafValue: leaf1Data, ExtraData: []byte("extra")}
	leaf3              = &trillian.LogLeaf{LeafIndex: 3, MerkleLeafHash: th.HashLeaf(leaf3Data), LeafValue: leaf3Data, ExtraData: []byte("extra3")}

	queueRequest0     = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{leaf1}}
	queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logID2, Leaves: []*trillian.LogLeaf{leaf1}}
	queueRequestEmpty = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{}}

	getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logID1}
	getLogRootRequest2 = trillian.GetLatestSignedLogRootRequest{LogId: logID2}
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

	getConsistencyProofRequest25 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 10, SecondTreeSize: 25}
	getConsistencyProofRequest7  = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 7}
	getConsistencyProofRequest44 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 4}
	getConsistencyProofRequest48 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 8}
	getConsistencyProofRequest54 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 5, SecondTreeSize: 4}

	nodeIdsInclusionSize7Index2 = []storage.NodeID{
		stestonly.MustCreateNodeIDForTreeCoords(0, 3, 64),
		stestonly.MustCreateNodeIDForTreeCoords(1, 0, 64),
		stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}

	nodeIdsConsistencySize4ToSize7 = []storage.NodeID{stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64)}
)

func TestGetLeavesByIndexInvalidIndexRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := extension.Registry{}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if _, err := server.GetLeavesByIndex(context.Background(), &leaf0Minus2Request); err != nil {
		t.Fatalf("Returned non app level error response for negative leaf index: %v", err)
	}
}

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

func TestQueueLeavesNoLeavesRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := extension.Registry{}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if _, err := server.QueueLeaves(context.Background(), &queueRequestEmpty); err == nil {
		t.Fatal("Allowed zero leaves to be queued")
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

func TestGetLatestSignedLogRootBeginFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getLogRootRequest1.LogId).Return(nil, errors.New("TX"))

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getLogRootRequest1.LogId),
		LogStorage:   mockStorage,
	}
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
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(trillian.SignedLogRoot{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeStorageFailureTest(t, getLogRootRequest1.LogId)
}

func TestGetLatestSignedLogRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(trillian.SignedLogRoot{}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
			return err
		})

	test.executeCommitFailsTest(t, getLogRootRequest1.LogId)
}

func TestGetLatestSignedLogRootInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "LatestSignedLogRoot", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest2)
			return err
		})

	test.executeInvalidLogIDTest(t, true /* snapshot */)
}

func TestGetLatestSignedLogRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getLogRootRequest1.LogId).Return(mockTx, nil)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getLogRootRequest1.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	resp, err := server.GetLatestSignedLogRoot(context.Background(), &getLogRootRequest1)
	if err != nil {
		t.Fatalf("Failed to get log root: %v", err)
	}

	if !proto.Equal(&signedRoot1, resp.SignedLogRoot) {
		t.Fatalf("Log root proto mismatch:\n%v\n%v", signedRoot1, resp.SignedLogRoot)
	}
}

func TestGetLeavesByHashInvalidHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := extension.Registry{}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	for _, test := range []struct {
		req     *trillian.GetLeavesByHashRequest
		wantErr bool
	}{
		{
			// This request includes an empty hash, which isn't allowed
			req: &trillian.GetLeavesByHashRequest{
				LogId:    logID1,
				LeafHash: [][]byte{[]byte(""), []byte("data")},
			},
			wantErr: true,
		},
	} {
		_, err := server.GetLeavesByHash(context.Background(), test.req)
		if got := err != nil; got != test.wantErr {
			t.Errorf("GetLeavesByHash(%v): %v, wantErr: %v", test.req, err, test.wantErr)
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

func TestGetConsistencyProofBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTreeTX) {},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest25)
			return err
		})

	test.executeBeginFailsTest(t, getConsistencyProofRequest25.LogId)
}

func TestGetConsistencyProofGetNodesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsConsistencySize4ToSize7).Return([]storage.Node{}, errors.New("STORAGE"))
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)
			return err
		})

	test.executeStorageFailureTest(t, getConsistencyProofRequest7.LogId)
}

func TestGetConsistencyProofGetNodesReturnsWrongCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getConsistencyProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	// The server expects one node from storage but we return two
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getConsistencyProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
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
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getConsistencyProofRequest7.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
	// Return an unexpected node that wasn't requested
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(1, 2, 64), NodeRevision: 3}}, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getConsistencyProofRequest7.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)
	if err == nil || !strings.Contains(err.Error(), "expected node ") {
		t.Fatalf("get consistency proof returned no or wrong error when get nodes returns wrong node: %v", err)
	}
}

func TestGetConsistencyProofCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetConsistencyProof", readOnly,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			t.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
			t.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsConsistencySize4ToSize7).Return([]storage.Node{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3}}, nil)
		},
		func(s *TrillianLogRPCServer) error {
			_, err := s.GetConsistencyProof(context.Background(), &getConsistencyProofRequest7)
			return err
		})

	test.executeCommitFailsTest(t, getConsistencyProofRequest7.LogId)
}

// Ask for a proof from size 4 to 8 but the tree is only size 7. This should fail.
func TestGetConsistencyProofOutsideTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), getConsistencyProofRequest48.LogId).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getConsistencyProofRequest48.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest48)
	if err == nil {
		t.Fatal("incorrectly returned a proof for out of range tree size")
	}
}

// Ask for a proof from size 5 to 4, testing the boundary condition. This request should fail
// before making any storage requests.
func TestGetConsistencyProofRangeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	registry := extension.Registry{
		AdminStorage: mockAdminStorage(ctrl, getConsistencyProofRequest54.LogId),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	_, err := server.GetConsistencyProof(context.Background(), &getConsistencyProofRequest54)
	if err == nil {
		t.Fatal("incorrectly returned a proof for out of range tree size")
	}
}

func TestGetConsistencyProof(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// getConsistencyProofRequest7 - tests a normal request which should succeed
	// getConsistencyProofRequest44 - tests an edge condition we used to reject but now support
	// for compatibilty with the older C++ log servers.
	testCases := []trillian.GetConsistencyProofRequest{getConsistencyProofRequest7, getConsistencyProofRequest44}
	nodeIDs := [][]storage.NodeID{nodeIdsConsistencySize4ToSize7, {}}
	hashes := [][][]byte{{[]byte("nodehash")}, {}}
	nodes := [][]storage.Node{{{NodeID: stestonly.MustCreateNodeIDForTreeCoords(2, 1, 64), NodeRevision: 3, Hash: []byte("nodehash")}}, {}}

	for i, _ := range testCases {
		mockStorage := storage.NewMockLogStorage(ctrl)
		mockTx := storage.NewMockLogTreeTX(ctrl)
		mockStorage.EXPECT().SnapshotForTree(gomock.Any(), testCases[i].LogId).Return(mockTx, nil)

		mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
		mockTx.EXPECT().ReadRevision().Return(signedRoot1.TreeRevision)
		mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIDs[i]).Return(nodes[i], nil)
		mockTx.EXPECT().Commit().Return(nil)
		mockTx.EXPECT().Close().Return(nil)

		registry := extension.Registry{
			AdminStorage: mockAdminStorage(ctrl, testCases[i].LogId),
			LogStorage:   mockStorage,
		}
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		response, err := server.GetConsistencyProof(context.Background(), &testCases[i])
		if err != nil {
			t.Fatalf("failed to get consistency proof: %v", err)
		}

		// Ensure we got the expected proof
		expectedProof := trillian.Proof{
			LeafIndex: 0,
			Hashes:    hashes[i],
		}
		if !proto.Equal(response.Proof, &expectedProof) {
			t.Fatalf("expected proof: %v but got: %v", expectedProof, response.Proof)
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

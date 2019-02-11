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
	"crypto"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/types"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcrypto "github.com/google/trillian/crypto"
	stestonly "github.com/google/trillian/storage/testonly"
)

func newTestLeaf(data []byte, extra []byte, index int64) *trillian.LogLeaf {
	hash, _ := th.HashLeaf(data)
	return &trillian.LogLeaf{
		MerkleLeafHash: hash,
		LeafValue:      data,
		ExtraData:      extra,
		LeafIndex:      index,
	}
}

var (
	th = rfc6962.DefaultHasher

	logID1 = int64(1)
	logID2 = int64(2)
	logID3 = int64(3)

	leaf1 = newTestLeaf([]byte("value"), []byte("extra"), 1)
	leaf2 = newTestLeaf([]byte("value2"), []byte("extra"), 2)
	leaf3 = newTestLeaf([]byte("value3"), []byte("extra3"), 3)

	leaf0Request     = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0}}
	leaf03Request    = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, 3}}
	leaf0Log2Request = trillian.GetLeavesByIndexRequest{LogId: logID2, LeafIndex: []int64{0}}

	queueRequest0     = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{leaf1}}
	queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logID2, Leaves: []*trillian.LogLeaf{leaf1}}

	addSeqRequest0 = trillian.AddSequencedLeavesRequest{LogId: logID3, Leaves: []*trillian.LogLeaf{leaf1}}

	fixedGoSigner = newSignerWithFixedSig([]byte("signed"))
	fixedSigner   = tcrypto.NewSigner(0, fixedGoSigner, crypto.SHA256)

	tree1              = addTreeID(stestonly.LogTree, logID1)
	getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logID1}
	revision1          = int64(5)
	root1              = &types.LogRootV1{TimestampNanos: 987654321, RootHash: []byte("A NICE HASH"), TreeSize: 7, Revision: uint64(revision1)}
	signedRoot1, _     = fixedSigner.SignLogRoot(root1)

	getByHashRequest1 = trillian.GetLeavesByHashRequest{LogId: logID1, LeafHash: [][]byte{[]byte("test"), []byte("data")}}
	getByHashRequest2 = trillian.GetLeavesByHashRequest{LogId: logID2, LeafHash: [][]byte{[]byte("test"), []byte("data")}}

	getInclusionProofByHashRequest7  = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 7, LeafHash: []byte("ahash")}
	getInclusionProofByHashRequest25 = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 25, LeafHash: []byte("ahash")}

	getInclusionProofByIndexRequest7  = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}
	getInclusionProofByIndexRequest25 = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: 25}

	getEntryAndProofRequest17    = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 3}
	getEntryAndProofRequest17_2  = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 2}
	getEntryAndProofRequest17_11 = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 11}
	getEntryAndProofRequest7     = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}

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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(nil, errors.New("TX"))
	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetLeavesByIndex", readOnly, nopStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)
	mockTX.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0, 3}).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)
	mockTX.EXPECT().IsOpen().AnyTimes().Return(false)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf03Request.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

func TestGetLeavesByRange(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeAdmin := storage.NewMockAdminStorage(ctrl)
	tree := &trillian.Tree{TreeId: 6962, TreeType: trillian.TreeType_LOG, TreeState: trillian.TreeState_ACTIVE}

	var tests = []struct {
		start, count int64
		skipTX       bool
		adminErr     error
		txErr        error
		getErr       error
		want         []*trillian.LogLeaf
		wantErr      string
	}{
		{
			start:    1,
			count:    1,
			adminErr: errors.New("admin_err"),
			wantErr:  "admin_err",
		},
		{
			start: 1,
			count: 1,
			want:  []*trillian.LogLeaf{leaf1},
		},
		{
			start:   1,
			count:   1,
			txErr:   errors.New("test error xyzzy"),
			wantErr: "test error xyzzy",
		},
		{
			start:   1,
			count:   1,
			getErr:  errors.New("test error plugh"),
			wantErr: "test error plugh",
		},
		{
			start: 1,
			count: 3,
			want:  []*trillian.LogLeaf{leaf1, leaf2, leaf3},
		},
		{
			start: 1,
			count: 30,
			want:  []*trillian.LogLeaf{leaf1, leaf2, leaf3},
		},
		{
			start:   -1,
			count:   1,
			skipTX:  true,
			wantErr: "want >= 0",
		},
		{
			start:   1,
			count:   0,
			skipTX:  true,
			wantErr: "want > 0",
		},
		{
			start:   1,
			count:   -1,
			skipTX:  true,
			wantErr: "want > 0",
		},
	}

	for _, test := range tests {
		if !test.skipTX {
			mockTX := storage.NewMockLogTreeTX(ctrl)
			mockAdminTX := storage.NewMockAdminTX(ctrl)
			mockAdminTX.EXPECT().GetTree(gomock.Any(), tree.TreeId).Return(tree, test.adminErr)
			mockAdminTX.EXPECT().Close().Return(nil)
			fakeAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTX, nil)
			if test.adminErr == nil {
				mockAdminTX.EXPECT().Commit().Return(nil)
				if test.txErr != nil {
					fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree).Return(nil, test.txErr)
				} else {
					fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree).Return(mockTX, nil)
					mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
					if test.getErr != nil {
						mockTX.EXPECT().GetLeavesByRange(gomock.Any(), test.start, test.count).Return(nil, test.getErr)
					} else {
						mockTX.EXPECT().GetLeavesByRange(gomock.Any(), test.start, test.count).Return(test.want, nil)
						mockTX.EXPECT().Commit().Return(nil)
					}
					mockTX.EXPECT().Close().Return(nil)
				}
			} else {
				if test.txErr != nil {
					mockTX.EXPECT().Commit().Return(nil)
				}
			}
		}
		registry := extension.Registry{LogStorage: fakeStorage, AdminStorage: fakeAdmin}
		server := NewTrillianLogRPCServer(registry, fakeTimeSource)

		req := trillian.GetLeavesByRangeRequest{
			LogId:      tree.TreeId,
			StartIndex: test.start,
			Count:      test.count,
		}
		rsp, err := server.GetLeavesByRange(ctx, &req)
		if err != nil {
			if test.wantErr == "" {
				t.Errorf("GetLeavesByRange(%d, %+d)=nil,%v; want _,nil", req.StartIndex, req.Count, err)
			} else if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("GetLeavesByRange(%d, %+d)=nil,%v; want _, err containing %q", req.StartIndex, req.Count, err, test.wantErr)
			}
			continue
		}
		if test.wantErr != "" {
			t.Errorf("GetLeavesByRange(%d, %+d)=_,nil; want nil, err containing %q", req.StartIndex, req.Count, test.wantErr)
		}
		if got := rsp.Leaves; !reflect.DeepEqual(got, test.want) {
			t.Errorf("GetLeavesByRange(%d, %+d)=%+v; want %+v", req.StartIndex, req.Count, got, test.want)
		}
	}
}

func TestQueueLeavesStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", noTX,
		func(s *stestonly.FakeLogStorage) {
			s.QueueLeavesErr = errors.New("STORAGE")
		},
		nopTX,
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0)
			return err
		})

	test.executeStorageFailureTest(t, queueRequest0.LogId)
}

func TestQueueLeavesInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "QueueLeaves", noTX, nopStorage, nopTX,
		func(s *TrillianLogRPCServer) error {
			_, err := s.QueueLeaves(context.Background(), &queueRequest0Log2)
			return err
		})

	test.executeInvalidLogIDTest(t, false /* snapshot */)
}

func okQueuedLeaf(l *trillian.LogLeaf) *trillian.QueuedLogLeaf {
	return &trillian.QueuedLogLeaf{
		Leaf:   l,
		Status: status.New(codes.OK, "OK").Proto(),
	}
}
func dupeQueuedLeaf(l *trillian.LogLeaf) *trillian.QueuedLogLeaf {
	return &trillian.QueuedLogLeaf{
		Leaf:   l,
		Status: status.New(codes.AlreadyExists, "Seen this before, mate").Proto(),
	}
}

func TestQueueLeaves(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockLogStorage(ctrl)
	c1 := mockStorage.EXPECT().QueueLeaves(gomock.Any(), tree1, []*trillian.LogLeaf{leaf1}, fakeTime).Return([]*trillian.QueuedLogLeaf{okQueuedLeaf(leaf1)}, nil)
	mockStorage.EXPECT().QueueLeaves(gomock.Any(), tree1, []*trillian.LogLeaf{leaf1}, fakeTime).After(c1).Return([]*trillian.QueuedLogLeaf{dupeQueuedLeaf(leaf1)}, nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: queueRequest0.LogId, numSnapshots: 2}),
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
	if queuedLeaf.Status.Code != int32(code.Code_OK) {
		t.Errorf("QueueLeaves().Status=%d,nil; want %d,nil", queuedLeaf.Status.Code, code.Code_OK)
	}
	if !proto.Equal(queueRequest0.Leaves[0], queuedLeaf.Leaf) {
		diff := pretty.Compare(queueRequest0.Leaves[0], queuedLeaf.Leaf)
		t.Errorf("post-QueueLeaves() diff:\n%v", diff)
	}

	// Repeating the operation gives ALREADY_EXISTS.
	rsp, err = server.QueueLeaves(ctx, &queueRequest0)
	if err != nil {
		t.Fatalf("Failed to re-queue leaf: %v", err)
	}
	if len(rsp.QueuedLeaves) != 1 {
		t.Errorf("QueueLeaves() returns %d leaves; want 1", len(rsp.QueuedLeaves))
	}
	queuedLeaf = rsp.QueuedLeaves[0]
	if queuedLeaf.Status == nil || queuedLeaf.Status.Code != int32(code.Code_ALREADY_EXISTS) {
		sc := "nil"
		if queuedLeaf.Status != nil {
			sc = fmt.Sprintf("%v", queuedLeaf.Status.Code)
		}
		t.Errorf("QueueLeaves().Status=%v,nil; want %v,nil", sc, code.Code_ALREADY_EXISTS)
	}
	if !proto.Equal(queueRequest0.Leaves[0], queuedLeaf.Leaf) {
		diff := pretty.Compare(queueRequest0.Leaves[0], queuedLeaf.Leaf)
		t.Errorf("post-QueueLeaves() diff:\n%v", diff)
	}
}

func TestAddSequencedLeavesStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "AddSequencedLeaves", noTX,
		func(s *stestonly.FakeLogStorage) {
			s.AddSequencedLeavesErr = errors.New("STORAGE")
		},
		nopTX,
		func(s *TrillianLogRPCServer) error {
			_, err := s.AddSequencedLeaves(context.Background(), &addSeqRequest0)
			return err
		})
	test.preordered = true

	test.executeStorageFailureTest(t, addSeqRequest0.LogId)
}

func TestAddSequencedLeavesInvalidLogId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "AddSequencedLeaves", noTX, nopStorage, nopTX,
		func(s *TrillianLogRPCServer) error {
			_, err := s.AddSequencedLeaves(context.Background(), &addSeqRequest0)
			return err
		})

	test.executeInvalidLogIDTest(t, false /* snapshot */)
}

func TestAddSequencedLeaves(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tree := addTreeID(stestonly.PreorderedLogTree, addSeqRequest0.LogId)
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockStorage.EXPECT().AddSequencedLeaves(gomock.Any(), tree, []*trillian.LogLeaf{leaf1}, gomock.Any()).
		Return([]*trillian.QueuedLogLeaf{{Status: status.New(codes.OK, "OK").Proto()}}, nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{addSeqRequest0.LogId, true, 1}),
		LogStorage:   mockStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	rsp, err := server.AddSequencedLeaves(ctx, &addSeqRequest0)
	if err != nil {
		t.Fatalf("Failed to add leaf: %v", err)
	}
	if len(rsp.Results) != 1 {
		t.Errorf("AddSequencedLeaves() returns %d leaves; want 1", len(rsp.Results))
	}
	result := rsp.Results[0]
	if got, want := result.Status.Code, int32(code.Code_OK); got != want {
		t.Errorf("AddSequencedLeaves().Status.Code=%d; want %d", got, want)
	}
	if result.Leaf != nil {
		t.Errorf("AddSequencedLeaves().Leaf=%v; want nil", result.Leaf)
	}
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
			wantRoot:    trillian.GetLatestSignedLogRootResponse{SignedLogRoot: signedRoot1},
			storageRoot: *signedRoot1,
		},
	}

	for _, test := range tests {
		fakeStorage := storage.NewMockLogStorage(ctrl)
		mockTX := storage.NewMockLogTreeTX(ctrl)
		if !test.noSnap {
			fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, test.snapErr)
		}
		if !test.noRoot {
			mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(test.storageRoot, test.rootErr)
		}
		if !test.noCommit {
			mockTX.EXPECT().Commit().Return(test.commitErr)
		}
		if !test.noClose {
			mockTX.EXPECT().Close().Return(nil)
		}

		registry := extension.Registry{
			AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: test.req.LogId, numSnapshots: 1}),
			LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(nil, errors.New("TX"))
	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getByHashRequest1.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("test"), []byte("data")}, false).Return(nil, nil)
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
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

	test := newParameterizedTest(ctrl, "GetLeavesByHash", readOnly, nopStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)
	mockTX.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("test"), []byte("data")}, false).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getByHashRequest1.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
			t.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// The server expects three nodes from storage but we return only two
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
	// We set this up so one of the returned nodes has the wrong ID
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: stestonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProofByHash", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
			t.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
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
	ctx := context.Background()
	for _, tc := range []struct {
		desc            string
		wantCode        codes.Code
		leavesByHashVal []*trillian.LogLeaf
	}{
		{desc: "OK", leavesByHashVal: []*trillian.LogLeaf{{LeafIndex: 2}}},
		{desc: "NotFoundTreeSize", wantCode: codes.NotFound, leavesByHashVal: []*trillian.LogLeaf{{LeafIndex: 7}}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			fakeStorage := storage.NewMockLogStorage(ctrl)
			mockTX := storage.NewMockLogTreeTX(ctrl)
			fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

			mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
			mockTX.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{[]byte("ahash")}, false).Return(tc.leavesByHashVal, nil)
			mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil).AnyTimes()
			mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
				{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
				{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
				{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil).AnyTimes()
			mockTX.EXPECT().Commit().Return(nil)
			mockTX.EXPECT().Close().Return(nil)

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{
					treeID:       logID1,
					numSnapshots: 1,
				}),
				LogStorage: fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)

			proofResponse, err := server.GetInclusionProofByHash(ctx,
				&trillian.GetInclusionProofByHashRequest{
					LogId:    logID1,
					TreeSize: 7,
					LeafHash: []byte("ahash"),
				})
			if got, want := status.Code(err), tc.wantCode; got != want {
				t.Fatalf("GetInclusionProofByHash(): %v, want %v", err, want)
			}
			if err != nil {
				return
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
		})
	}
}

func TestGetProofByIndexBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
			t.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	// The server expects three nodes from storage but we return only two
	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeRevision: 3}, {NodeRevision: 2}}, nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	// We set this up so one of the returned nodes has the wrong ID
	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3}, {NodeID: stestonly.MustCreateNodeIDForTreeCoords(4, 5, 64), NodeRevision: 2}, {NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3}}, nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	test := newParameterizedTest(ctrl, "GetInclusionProof", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
			t.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest17.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

func TestGetProofByIndexBeyondSTH(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest17.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	proofResponse, err := server.GetInclusionProof(context.Background(), &getInclusionProofByIndexRequest25)
	if err != nil {
		t.Fatalf("get inclusion proof by index should have succeeded but we got: %v", err)
	}

	if proofResponse == nil {
		t.Fatalf("server response was not successful: %v", proofResponse)
	}

	if proofResponse.Proof != nil {
		t.Fatalf("expected nil proof but got: %v", proofResponse.Proof)
	}
}

func TestGetEntryAndProofBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(nil, errors.New("BeginTX"))
	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest17.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{}, errors.New("GetNodes"))
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return(nil, errors.New("GetLeaves"))
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	// Code passed one leaf index so expects one result, but we return more
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTX.EXPECT().Commit().Return(errors.New("COMMIT"))
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
	mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTX.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest7.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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

func TestGetEntryAndProofSkewNoProof(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest17_11.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest17_11)
	if err != nil {
		t.Errorf("get entry and proof should have succeeded but we got: %v", err)
	}

	if response.Proof != nil {
		t.Errorf("expected nil proof but got: %v", response.Proof)
	}

	if response.Leaf != nil {
		t.Fatalf("Expected nil leaf but got: %v", response.Leaf)
	}
}

func TestGetEntryAndProofSkewSmallerTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTx, nil)

	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
	mockTx.EXPECT().ReadRevision(gomock.Any()).Return(signedRoot1.TreeRevision, nil) // nolint: megacheck
	mockTx.EXPECT().GetMerkleNodes(gomock.Any(), revision1, nodeIdsInclusionSize7Index2).Return([]storage.Node{
		{NodeID: nodeIdsInclusionSize7Index2[0], NodeRevision: 3, Hash: []byte("nodehash0")},
		{NodeID: nodeIdsInclusionSize7Index2[1], NodeRevision: 2, Hash: []byte("nodehash1")},
		{NodeID: nodeIdsInclusionSize7Index2[2], NodeRevision: 3, Hash: []byte("nodehash2")}}, nil)
	mockTx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{2}).Return([]*trillian.LogLeaf{leaf1}, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: getEntryAndProofRequest17_2.LogId, numSnapshots: 1}),
		LogStorage:   fakeStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	response, err := server.GetEntryAndProof(context.Background(), &getEntryAndProofRequest17_2)
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

	// Check we got the right signed log root
	if !proto.Equal(response.SignedLogRoot, signedRoot1) {
		t.Fatalf("expected root: %v got: %v", signedRoot1, response.SignedLogRoot)
	}
}

func TestGetSequencedLeafCountBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly, nopStorage,
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

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly, nopStorage,
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

	fakeStorage := storage.NewMockLogStorage(ctrl)
	mockTX := storage.NewMockLogTreeTX(ctrl)
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, nil)

	mockTX.EXPECT().GetSequencedLeafCount(gomock.Any()).Return(int64(268), nil)
	mockTX.EXPECT().Commit().Return(nil)
	mockTX.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: logID1, numSnapshots: 1}),
		LogStorage:   fakeStorage,
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
			// Ask for a proof from size 4 to 8 but the tree is only size 7. This should succeed but with no proof.
			req:        getConsistencyProofRequest48,
			wantHashes: nil,
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
		fakeStorage := storage.NewMockLogStorage(ctrl)
		mockTX := storage.NewMockLogTreeTX(ctrl)
		if !test.noSnap {
			fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(mockTX, test.snapErr)
		}
		if !test.noRoot {
			mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, test.rootErr)
		}
		if !test.noRev {
			mockTX.EXPECT().ReadRevision(gomock.Any()).Return(int64(root1.Revision), nil)
		}
		if test.nodeIDs != nil {
			mockTX.EXPECT().GetMerkleNodes(gomock.Any(), revision1, test.nodeIDs).Return(test.nodes, test.getNodesErr)
		}
		if !test.noCommit {
			mockTX.EXPECT().Commit().Return(test.commitErr)
		}
		if !test.noClose {
			mockTX.EXPECT().Close().Return(nil)
		}

		registry := extension.Registry{
			AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: test.req.LogId, numSnapshots: 1}),
			LogStorage:   fakeStorage,
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
			if test.wantHashes == nil {
				if response.Proof != nil {
					t.Errorf("GetConsistencyProof(%+v) want nil proof, got %v", test.req, response.Proof)
				}
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

func TestInitLog(t *testing.T) {
	ctx := context.Background()
	// A non-empty log root
	signedRoot, err := fixedSigner.SignLogRoot(&types.LogRootV1{})
	if err != nil {
		t.Fatalf("SignedLogRoot(): %v", err)
	}

	for _, tc := range []struct {
		desc       string
		preordered bool
		getRootErr error
		wantInit   bool
		slr        trillian.SignedLogRoot
		wantCode   codes.Code
	}{
		{desc: "init new log", getRootErr: storage.ErrTreeNeedsInit, wantInit: true, wantCode: codes.OK},
		{desc: "init new preordered log", preordered: true, getRootErr: storage.ErrTreeNeedsInit, wantInit: true, wantCode: codes.OK},
		{desc: "init new log, no err", getRootErr: nil, wantInit: true, wantCode: codes.OK},
		{desc: "init already initialised log", getRootErr: nil, wantInit: false, slr: *signedRoot, wantCode: codes.AlreadyExists},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTX := storage.NewMockLogTreeTX(ctrl)
			fakeStorage := &stestonly.FakeLogStorage{TX: mockTX}
			if tc.getRootErr != nil {
				mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(trillian.SignedLogRoot{}, tc.getRootErr)
			} else {

				mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(tc.slr, nil)
			}
			mockTX.EXPECT().IsOpen().AnyTimes().Return(false)
			mockTX.EXPECT().Close().Return(nil)
			if tc.wantInit {
				mockTX.EXPECT().Commit().Return(nil)
				mockTX.EXPECT().StoreSignedLogRoot(gomock.Any(), gomock.Any())
			}

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{logID1, tc.preordered, 1}),
				LogStorage:   fakeStorage,
			}
			logServer := NewTrillianLogRPCServer(registry, fakeTimeSource)

			c, err := logServer.InitLog(ctx, &trillian.InitLogRequest{LogId: logID1})
			if got, want := status.Code(err), tc.wantCode; got != want {
				t.Errorf("InitLog returned %v (%v), want %v", got, err, want)
			}
			if tc.wantInit {
				if err != nil {
					t.Fatalf("InitLog returned %v, want no error", err)
				}
				if c.Created == nil {
					t.Error("InitLog first attempt didn't return the created STH.")
				}
			} else {
				if err == nil {
					t.Errorf("InitLog returned nil, want error")
				}
			}
		})
	}
}

type prepareFakeStorageFunc func(*stestonly.FakeLogStorage)
type prepareMockTXFunc func(*storage.MockLogTreeTX)
type makeRPCFunc func(*TrillianLogRPCServer) error

type txMode int

func nopTX(*storage.MockLogTreeTX)         {}
func nopStorage(*stestonly.FakeLogStorage) {}

const (
	readOnly txMode = iota
	readWrite
	noTX
)

type parameterizedTest struct {
	ctrl       *gomock.Controller
	operation  string
	mode       txMode
	preordered bool

	prepareStorage prepareFakeStorageFunc
	prepareTX      prepareMockTXFunc
	makeRPC        makeRPCFunc
}

func newParameterizedTest(ctrl *gomock.Controller, operation string, m txMode, prepareStorage prepareFakeStorageFunc, prepareTx prepareMockTXFunc, makeRPC makeRPCFunc) *parameterizedTest {
	return &parameterizedTest{ctrl, operation, m, false /* preordered */, prepareStorage, prepareTx, makeRPC}
}

func (p *parameterizedTest) executeCommitFailsTest(t *testing.T, logID int64) {
	withRoot := false
	t.Helper()

	mockTX := storage.NewMockLogTreeTX(p.ctrl)
	fakeStorage := &stestonly.FakeLogStorage{}

	switch p.mode {
	case readOnly:
		fakeStorage.ReadOnlyTX = mockTX
	case readWrite:
		fakeStorage.TX = mockTX
	}
	if p.mode != noTX {
		p.prepareTX(mockTX)
		if withRoot {
			mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*signedRoot1, nil)
		}
		mockTX.EXPECT().Commit().Return(errors.New("bang"))
		mockTX.EXPECT().Close().Return(errors.New("bang"))
		mockTX.EXPECT().IsOpen().AnyTimes().Return(false)
	}

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1}),
		LogStorage:   fakeStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	err := p.makeRPC(server)
	if err == nil {
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

	fakeStorage := storage.NewMockLogStorage(p.ctrl)
	if ctx := gomock.Any(); snapshot {
		fakeStorage.EXPECT().SnapshotForTree(ctx, tree1).MaxTimes(1).Return(nil, badLogErr)
	}

	registry := extension.Registry{
		AdminStorage: adminStorage,
		LogStorage:   fakeStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	// Make a request for a nonexistent log id
	if err := p.makeRPC(server); err == nil || !strings.Contains(err.Error(), badLogErr.Error()) {
		t.Fatalf("Returned wrong error response for nonexistent log: %s: %v", p.operation, err)
	}
}

func (p *parameterizedTest) executeStorageFailureTest(t *testing.T, logID int64) {
	fakeStorage := &stestonly.FakeLogStorage{}
	mockTX := storage.NewMockLogTreeTX(p.ctrl)
	mockTX.EXPECT().Close().AnyTimes()

	p.prepareStorage(fakeStorage)
	switch p.mode {
	case readOnly:
		fakeStorage.ReadOnlyTX = mockTX
	case readWrite:
		fakeStorage.TX = mockTX
	}
	if p.mode != noTX {
		p.prepareTX(mockTX)
	}

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1}),
		LogStorage:   fakeStorage,
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
		logStorage.EXPECT().SnapshotForTree(gomock.Any(), tree1).Return(logTX, errors.New("TX"))
	case readWrite:
		logStorage.EXPECT().ReadWriteTransaction(gomock.Any(), logID, gomock.Any()).Return(errors.New("TX"))
	}

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1}),
		LogStorage:   logStorage,
	}
	server := NewTrillianLogRPCServer(registry, fakeTimeSource)

	if err := p.makeRPC(server); err == nil || !strings.Contains(err.Error(), "TX") {
		t.Fatalf("Returned wrong error response when begin failed: %v", err)
	}
}

type storageParams struct {
	treeID       int64
	preordered   bool
	numSnapshots int
}

func fakeAdminStorage(ctrl *gomock.Controller, params storageParams) storage.AdminStorage {
	tree := *stestonly.LogTree
	if params.preordered {
		tree = *stestonly.PreorderedLogTree
	}
	tree.TreeId = params.treeID

	adminStorage := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)

	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(params.numSnapshots).Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), params.treeID).MaxTimes(params.numSnapshots).Return(&tree, nil)
	adminTX.EXPECT().Close().MaxTimes(params.numSnapshots).Return(nil)
	adminTX.EXPECT().Commit().MaxTimes(params.numSnapshots).Return(nil)

	return adminStorage
}

func addTreeID(tree *trillian.Tree, treeID int64) *trillian.Tree {
	newTree := proto.Clone(tree).(*trillian.Tree)
	newTree.TreeId = treeID
	return newTree
}

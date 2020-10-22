// Copyright 2016 Google LLC. All Rights Reserved.
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
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/compact"
	_ "github.com/google/trillian/merkle/rfc6962" // Register the hasher.
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// cmpMatcher is a custom gomock.Matcher that uses cmp.Equal combined with a
// cmp.Comparer that knows how to properly compare proto.Message types.
type cmpMatcher struct{ want interface{} }

func (m cmpMatcher) Matches(got interface{}) bool {
	return cmp.Equal(got, m.want, cmp.Comparer(proto.Equal))
}

func (m cmpMatcher) String() string {
	return fmt.Sprintf("equals %v", m.want)
}

func newTestLeaf(data []byte, extra []byte, index int64) *trillian.LogLeaf {
	hash := th.HashLeaf(data)
	return &trillian.LogLeaf{
		MerkleLeafHash: hash,
		LeafValue:      data,
		ExtraData:      extra,
		LeafIndex:      index,
	}
}

var (
	th = rfc6962.DefaultHasher

	fakeTime       = time.Date(2016, 5, 25, 10, 55, 5, 0, time.UTC)
	fakeTimeSource = clock.NewFake(fakeTime)

	logID1 = int64(1)
	logID2 = int64(2)
	logID3 = int64(3)

	leaf1     = newTestLeaf([]byte("value"), []byte("extra"), 1)
	leafHash1 = []byte("\xcd\x42\x40\x4d\x52\xad\x55\xcc\xfa\x9a\xca\x4a\xdc\x82\x8a\xa5\x80\x0a\xd9\xd3\x85\xa0\x67\x1f\xbc\xbf\x72\x41\x18\x32\x06\x19")
	leaf2     = newTestLeaf([]byte("value2"), []byte("extra"), 2)
	leafHash2 = []byte("\x05\x37\xd4\x81\xf7\x3a\x75\x73\x34\x32\x80\x52\xda\x3a\xf9\x62\x6c\xed\x97\x02\x8e\x20\xb8\x49\xf6\x11\x5c\x22\xcd\x76\x51\x97")
	leaf3     = newTestLeaf([]byte("value3"), []byte("extra3"), 3)

	leaf0Request  = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0}}
	leaf03Request = trillian.GetLeavesByIndexRequest{LogId: logID1, LeafIndex: []int64{0, 3}}

	queueRequest0     = trillian.QueueLeavesRequest{LogId: logID1, Leaves: []*trillian.LogLeaf{leaf1}}
	queueRequest0Log2 = trillian.QueueLeavesRequest{LogId: logID2, Leaves: []*trillian.LogLeaf{leaf1}}

	addSeqRequest0 = trillian.AddSequencedLeavesRequest{LogId: logID3, Leaves: []*trillian.LogLeaf{leaf1}}

	fixedGoSigner = newSignerWithFixedSig([]byte("signed"))
	fixedSigner   = tcrypto.NewSigner(0, fixedGoSigner, crypto.SHA256)

	tree1              = addTreeID(stestonly.LogTree, logID1)
	getLogRootRequest1 = trillian.GetLatestSignedLogRootRequest{LogId: logID1}
	root1              = &types.LogRootV1{TimestampNanos: 987654321, RootHash: []byte("A NICE HASH"), TreeSize: 7, Revision: uint64(5)}
	signedRoot1, _     = fixedSigner.SignLogRoot(root1)

	getInclusionProofByHashRequest7  = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 7, LeafHash: leafHash1}
	getInclusionProofByHashRequest25 = trillian.GetInclusionProofByHashRequest{LogId: logID1, TreeSize: 25, LeafHash: leafHash2}

	getInclusionProofByIndexRequest7  = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}
	getInclusionProofByIndexRequest25 = trillian.GetInclusionProofRequest{LogId: logID1, TreeSize: 50, LeafIndex: 25}

	getEntryAndProofRequest17    = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 3}
	getEntryAndProofRequest17_2  = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 2}
	getEntryAndProofRequest17_11 = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 17, LeafIndex: 11}
	getEntryAndProofRequest7     = trillian.GetEntryAndProofRequest{LogId: logID1, TreeSize: 7, LeafIndex: 2}

	getConsistencyProofRequest7  = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 7}
	getConsistencyProofRequest44 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 4}
	getConsistencyProofRequest48 = trillian.GetConsistencyProofRequest{LogId: logID1, FirstTreeSize: 4, SecondTreeSize: 8}

	nodeIdsInclusionSize7Index2 = []compact.NodeID{
		compact.NewNodeID(0, 3),
		compact.NewNodeID(1, 0),
		compact.NewNodeID(2, 1),
	}

	nodeIdsConsistencySize4ToSize7 = []compact.NodeID{compact.NewNodeID(2, 1)}
	corruptLogRoot                 = &trillian.SignedLogRoot{LogRoot: []byte("this is not tls encoded data")}
)

func TestGetLeavesByIndex(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setupStorage func(*gomock.Controller, *storage.MockLogStorage)
		snapErr      error
		treeErr      error
		req          *trillian.GetLeavesByIndexRequest
		errStr       string
		wantResp     *trillian.GetLeavesByIndexResponse
	}{
		{
			name: "admin snapshot fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &leaf0Request,
			snapErr: errors.New("admin snap"),
			errStr:  "admin snap",
		},
		{
			name: "get tree fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &leaf0Request,
			treeErr: errors.New("tree error"),
			errStr:  "tree error",
		},
		{
			name: "begin fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(nil, errors.New("TX"))
			},
			req:    &leaf0Request,
			errStr: "TX",
		},
		{
			name: "not initialized",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, storage.ErrTreeNeedsInit)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &leaf0Request,
			errStr: "tree needs init",
		},
		{
			name: "storage error",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return(nil, errors.New("STORAGE"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &leaf0Request,
			errStr: "STORAGE",
		},
		{
			name: "commit fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(errors.New("COMMIT"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &leaf0Request,
			errStr: "COMMIT",
		},
		{
			name: "log root fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, errors.New("SLR"))
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &leaf0Request,
			errStr: "SLR",
		},
		{
			name: "bad log root",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(corruptLogRoot, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &leaf0Request,
			errStr: "not read current log root",
		},
		{
			name: "ok",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0}).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req: &leaf0Request,
			wantResp: &trillian.GetLeavesByIndexResponse{
				SignedLogRoot: signedRoot1,
				Leaves:        []*trillian.LogLeaf{leaf1},
			},
		},
		{
			name: "ok multiple",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByIndex(gomock.Any(), []int64{0, 3}).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req: &leaf03Request,
			wantResp: &trillian.GetLeavesByIndexResponse{
				SignedLogRoot: signedRoot1,
				Leaves:        []*trillian.LogLeaf{leaf1, leaf3},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			fakeStorage := storage.NewMockLogStorage(ctrl)
			tc.setupStorage(ctrl, fakeStorage)
			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1, snapErr: tc.snapErr, treeErr: tc.treeErr}),
				LogStorage:   fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)
			resp, err := server.GetLeavesByIndex(context.Background(), tc.req)
			if len(tc.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), tc.errStr) {
					t.Errorf("GetLeavesByIndex(%v)=%v, %v want nil, err containing: %s", tc.req, resp, err, tc.errStr)
				}
				return
			}

			if err != nil || !proto.Equal(tc.wantResp, resp) {
				t.Errorf("GetLeavesByIndex(%v)=%v, %v, want: %v, nil", tc.req, resp, err, tc.wantResp)
			}
		})
	}
}

func TestGetLeavesByRange(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	fakeStorage := storage.NewMockLogStorage(ctrl)
	fakeAdmin := storage.NewMockAdminStorage(ctrl)
	tree := &trillian.Tree{TreeId: 6962, TreeType: trillian.TreeType_LOG, TreeState: trillian.TreeState_ACTIVE}

	tests := []struct {
		start, count int64
		skipTX       bool
		adminErr     error
		txErr        error
		getErr       error
		slrErr       error
		root         *trillian.SignedLogRoot
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
			root:    signedRoot1,
			slrErr:  errors.New("SLR"),
			wantErr: "SLR",
		},
		{
			start:   1,
			count:   1,
			root:    corruptLogRoot,
			wantErr: "not read current log root",
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
					fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree}).Return(nil, test.txErr)
				} else {
					root := test.root
					if root == nil {
						root = signedRoot1
					}
					fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree}).Return(mockTX, nil)
					mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(root, test.slrErr)

					if test.root == nil {
						if test.getErr != nil {
							mockTX.EXPECT().GetLeavesByRange(gomock.Any(), test.start, test.count).Return(nil, test.getErr)
						} else {
							mockTX.EXPECT().GetLeavesByRange(gomock.Any(), test.start, test.count).Return(test.want, nil)
							mockTX.EXPECT().Commit(gomock.Any()).Return(nil)
						}
					}
					mockTX.EXPECT().Close().Return(nil)
				}
			} else {
				if test.txErr != nil {
					mockTX.EXPECT().Commit(gomock.Any()).Return(nil)
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

		if got := rsp.Leaves; !cmp.Equal(got, test.want, cmp.Comparer(proto.Equal)) {
			t.Errorf("GetLeavesByRange(%d, %+d)=%+v; want %+v", req.StartIndex, req.Count, got, test.want)
		}

		if gotCount, wantCount := server.fetchedLeaves.Value(), float64(len(test.want)); gotCount != wantCount {
			t.Errorf("GetLeavesByRange() incremented fetched count by %f,  want %f", gotCount, wantCount)
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
	c1 := mockStorage.EXPECT().QueueLeaves(gomock.Any(), cmpMatcher{tree1}, cmpMatcher{[]*trillian.LogLeaf{leaf1}}, fakeTime).Return([]*trillian.QueuedLogLeaf{okQueuedLeaf(leaf1)}, nil)
	mockStorage.EXPECT().QueueLeaves(gomock.Any(), cmpMatcher{tree1}, cmpMatcher{[]*trillian.LogLeaf{leaf1}}, fakeTime).After(c1).Return([]*trillian.QueuedLogLeaf{dupeQueuedLeaf(leaf1)}, nil)

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
		diff := cmp.Diff(queueRequest0.Leaves[0], queuedLeaf.Leaf)
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
		diff := cmp.Diff(queueRequest0.Leaves[0], queuedLeaf.Leaf)
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
	mockStorage.EXPECT().AddSequencedLeaves(gomock.Any(), cmpMatcher{tree}, cmpMatcher{[]*trillian.LogLeaf{leaf1}}, gomock.Any()).
		Return([]*trillian.QueuedLogLeaf{{Status: status.New(codes.OK, "OK").Proto()}}, nil)

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(ctrl, storageParams{addSeqRequest0.LogId, true, 1, nil, nil, false}),
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
	desc        string
	req         *trillian.GetLatestSignedLogRootRequest
	wantRoot    *trillian.GetLatestSignedLogRootResponse
	errStr      string
	noSnap      bool
	snapErr     error
	noRoot      bool
	storageRoot *trillian.SignedLogRoot
	rootErr     error
	noCommit    bool
	commitErr   error
	noClose     bool
}

func TestGetLatestSignedLogRoot(t *testing.T) {
	tests := []latestRootTest{
		{
			desc: "snap_fail",
			// Test error case when failing to get a snapshot from storage.
			req:      &getLogRootRequest1,
			snapErr:  errors.New("SnapshotForTree() error"),
			errStr:   "SnapshotFor",
			noRoot:   true,
			noCommit: true,
			noClose:  true,
		},
		{
			desc: "storage_fail",
			// Test error case when storage fails to provide a root.
			req:      &getLogRootRequest1,
			errStr:   "LatestSigned",
			rootErr:  errors.New("LatestSignedLogRoot() error"),
			noCommit: true,
		},
		{
			desc: "read_error",
			// Test error case where the log root could not be read
			req:      &getLogRootRequest1,
			errStr:   "rpc error: code = Internal desc = Could not read current log root: logRootBytes too short",
			noCommit: true,
		},
		{
			desc: "ok",
			// Test normal case where a root is returned correctly.
			req:         &getLogRootRequest1,
			wantRoot:    &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: signedRoot1},
			storageRoot: signedRoot1,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			fakeStorage := storage.NewMockLogStorage(ctrl)
			mockTX := storage.NewMockLogTreeTX(ctrl)
			if !test.noSnap {
				fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(mockTX, test.snapErr)
			}
			if !test.noRoot {
				mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(test.storageRoot, test.rootErr)
			}
			if !test.noCommit {
				mockTX.EXPECT().Commit(gomock.Any()).Return(test.commitErr)
			}
			if !test.noClose {
				mockTX.EXPECT().Close().Return(nil)
			}

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: test.req.LogId, numSnapshots: 1}),
				LogStorage:   fakeStorage,
			}
			s := NewTrillianLogRPCServer(registry, fakeTimeSource)
			got, err := s.GetLatestSignedLogRoot(context.Background(), test.req)
			if len(test.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), test.errStr) {
					t.Errorf("GetLatestSignedLogRoot(%+v)=_,nil, want: _,err contains: %s but got: %v", test.req, test.errStr, err)
				}
			} else {
				if err != nil {
					t.Errorf("GetLatestSignedLogRoot(%+v)=_,%v, want: _,nil", test.req, err)
					return
				}
				// Ensure we got the expected root back.
				if !proto.Equal(got.SignedLogRoot, test.wantRoot.SignedLogRoot) {
					t.Errorf("GetConsistencyProof(%+v)=%v,nil, want: %v,nil", test.req, got, test.wantRoot)
				}
			}
		})
	}
}

func TestGetProofByHashErrors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setupStorage func(*gomock.Controller, *storage.MockLogStorage)
		snapErr      error
		treeErr      error
		req          *trillian.GetInclusionProofByHashRequest
		errStr       string
		wantResp     *trillian.GetInclusionProofByHashResponse
	}{
		{
			name: "admin snapshot fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getInclusionProofByHashRequest25,
			snapErr: errors.New("admin snap"),
			errStr:  "admin snap",
		},
		{
			name: "get tree fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getInclusionProofByHashRequest25,
			treeErr: errors.New("tree error"),
			errStr:  "tree error",
		},
		{
			name: "begin fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(nil, errors.New("TX"))
			},
			req:    &getInclusionProofByHashRequest25,
			errStr: "TX",
		},
		{
			name: "not initialized",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, storage.ErrTreeNeedsInit)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest25,
			errStr: "tree needs init",
		},
		{
			name: "storage error",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash2}, false).Return(nil, errors.New("STORAGE"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest25,
			errStr: "STORAGE",
		},
		{
			name: "get nodes fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return(nil, errors.New("STORAGE"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "STORAGE",
		},
		{
			name: "too few nodes",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
				// The server expects three nodes from storage but we return only two
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{{}, {}}, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "expected 3 nodes",
		},
		{
			name: "wrong node",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return([]*trillian.LogLeaf{{LeafIndex: 2}}, nil)
				// We set this up so one of the returned nodes has the wrong ID
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{{ID: nodeIdsInclusionSize7Index2[0]}, {ID: compact.NewNodeID(4, 5)}, {ID: nodeIdsInclusionSize7Index2[2]}}, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "expected node ",
		},
		{
			name: "commit fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return(nil, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(errors.New("COMMIT"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "COMMIT",
		},
		{
			name: "log root fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return(nil, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(&trillian.SignedLogRoot{}, errors.New("SLR"))
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "SLR",
		},
		{
			name: "bad log root",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return(nil, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(corruptLogRoot, nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getInclusionProofByHashRequest7,
			errStr: "not read current log root",
		},
		{
			name: "leaf hash too short",
			req: &trillian.GetInclusionProofByHashRequest{
				LeafHash: []byte("too-short-to-be-a-hash"),
				LogId:    logID1,
				TreeSize: 7,
			},
			errStr: "GetInclusionProofByHashRequest.LeafHash: 22 bytes, want 32",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			fakeStorage := storage.NewMockLogStorage(ctrl)
			if tc.setupStorage != nil {
				tc.setupStorage(ctrl, fakeStorage)
			}
			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1, snapErr: tc.snapErr, treeErr: tc.treeErr}),
				LogStorage:   fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)
			resp, err := server.GetInclusionProofByHash(context.Background(), tc.req)
			if len(tc.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), tc.errStr) {
					t.Errorf("GetInclusionProofByHash(%v)=(%v, %v), want (nil, err containing %q)", tc.req, resp, err, tc.errStr)
				}
				return
			}

			if err != nil || !proto.Equal(tc.wantResp, resp) {
				t.Errorf("GetInclusionProofByHash(%v)=(%v, %v), want (%v, nil)", tc.req, resp, err, tc.wantResp)
			}
		})
	}
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
			fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(mockTX, nil)

			mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
			mockTX.EXPECT().GetLeavesByHash(gomock.Any(), [][]byte{leafHash1}, false).Return(tc.leavesByHashVal, nil)
			mockTX.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
				{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
				{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
				{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
			}, nil).AnyTimes()
			mockTX.EXPECT().Commit(gomock.Any()).Return(nil)
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
					LeafHash: leafHash1,
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

			expectedProof := &trillian.Proof{
				LeafIndex: 2,
				Hashes: [][]byte{
					[]byte("nodehash0"),
					[]byte("nodehash1"),
					[]byte("nodehash2"),
				},
			}

			if !proto.Equal(proofResponse.Proof[0], expectedProof) {
				t.Fatalf("expected proof: %v but got: %v", proto.CompactTextString(expectedProof), proto.CompactTextString(proofResponse.Proof[0]))
			}
		})
	}
}

func TestGetProofByIndex(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setupStorage func(*gomock.Controller, *storage.MockLogStorage)
		snapErr      error
		treeErr      error
		req          *trillian.GetInclusionProofRequest
		errStr       string
		wantResp     *trillian.GetInclusionProofResponse
	}{
		{
			name: "admin snapshot fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getInclusionProofByIndexRequest25,
			snapErr: errors.New("admin snap"),
			errStr:  "admin snap",
		},
		{
			name: "get tree fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getInclusionProofByIndexRequest25,
			treeErr: errors.New("tree error"),
			errStr:  "tree error",
		},
		{
			name: "begin fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(nil, errors.New("TX"))
			},
			req:    &getInclusionProofByIndexRequest25,
			errStr: "TX",
		},
		{
			name: "not initialized",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, storage.ErrTreeNeedsInit)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByIndexRequest25,
			errStr: "tree needs init",
		},
		{
			name: "get nodes fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return(nil, errors.New("STORAGE"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "STORAGE",
		},
		{
			name: "too few nodes",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				// The server expects three nodes from storage but we return only two
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{{}, {}}, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "expected 3 nodes",
		},
		{
			name: "wrong node",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				// We set this up so one of the returned nodes has the wrong ID
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{{ID: nodeIdsInclusionSize7Index2[0]}, {ID: compact.NewNodeID(4, 5)}, {ID: nodeIdsInclusionSize7Index2[2]}}, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "expected node ",
		},
		{
			name: "commit fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{{ID: nodeIdsInclusionSize7Index2[0]}, {ID: nodeIdsInclusionSize7Index2[1]}, {ID: nodeIdsInclusionSize7Index2[2]}}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(errors.New("COMMIT"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "COMMIT",
		},
		{
			name: "log root fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(&trillian.SignedLogRoot{}, errors.New("SLR"))
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "SLR",
		},
		{
			name: "bad log root",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(corruptLogRoot, nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getInclusionProofByIndexRequest7,
			errStr: "not read current log root",
		},
		{
			name: "ok",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
			},
			req: &getInclusionProofByIndexRequest7,
			wantResp: &trillian.GetInclusionProofResponse{
				SignedLogRoot: signedRoot1,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					Hashes: [][]byte{
						[]byte("nodehash0"),
						[]byte("nodehash1"),
						[]byte("nodehash2"),
					},
				},
			},
		},
		{
			name: "skew beyond sth",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req: &getInclusionProofByIndexRequest25,
			wantResp: &trillian.GetInclusionProofResponse{
				SignedLogRoot: signedRoot1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			fakeStorage := storage.NewMockLogStorage(ctrl)
			tc.setupStorage(ctrl, fakeStorage)
			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1, snapErr: tc.snapErr, treeErr: tc.treeErr}),
				LogStorage:   fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)
			resp, err := server.GetInclusionProof(context.Background(), tc.req)
			if len(tc.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), tc.errStr) {
					t.Errorf("GetInclusionProofByHash(%v)=%v, %v want nil, err containing: %s", tc.req, resp, err, tc.errStr)
				}
				return
			}

			if err != nil || !proto.Equal(tc.wantResp, resp) {
				t.Errorf("GetInclusionProofByHash(%v)=%v, %v, want: %v, nil", tc.req, resp, err, tc.wantResp)
			}
		})
	}
}

func TestGetEntryAndProof(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setupStorage func(*gomock.Controller, *storage.MockLogStorage)
		snapErr      error
		treeErr      error
		req          *trillian.GetEntryAndProofRequest
		errStr       string
		wantResp     *trillian.GetEntryAndProofResponse
	}{
		{
			name: "admin snapshot fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getEntryAndProofRequest17,
			snapErr: errors.New("admin snap"),
			errStr:  "admin snap",
		},
		{
			name: "get tree fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
			},
			req:     &getEntryAndProofRequest17,
			treeErr: errors.New("tree error"),
			errStr:  "tree error",
		},
		{
			name: "begin fails",
			setupStorage: func(_ *gomock.Controller, s *storage.MockLogStorage) {
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(nil, errors.New("TX"))
			},
			req:    &getEntryAndProofRequest17,
			errStr: "TX",
		},
		{
			name: "not initialized",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, storage.ErrTreeNeedsInit)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getEntryAndProofRequest17,
			errStr: "tree needs init",
		},
		{
			name: "storage error",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				tx.EXPECT().GetLeavesByRange(gomock.Any(), int64(2), int64(1)).Return(nil, errors.New("STORAGE"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getEntryAndProofRequest7,
			errStr: "STORAGE",
		},
		{
			name: "commit fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				tx.EXPECT().GetLeavesByRange(gomock.Any(), int64(2), int64(1)).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(errors.New("COMMIT"))
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getEntryAndProofRequest7,
			errStr: "COMMIT",
		},
		{
			name: "log root fails",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, errors.New("SLR"))
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getEntryAndProofRequest17,
			errStr: "SLR",
		},
		{
			name: "bad log root",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(corruptLogRoot, nil)
				tx.EXPECT().Close().Return(nil)
				tx.EXPECT().IsOpen().AnyTimes().Return(false)
			},
			req:    &getEntryAndProofRequest17,
			errStr: "not read current log root",
		},
		{
			name: "multiple leaves incorrect",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				// Code passed one leaf index so expects one result, but we return more
				tx.EXPECT().GetLeavesByRange(gomock.Any(), int64(2), int64(1)).Return([]*trillian.LogLeaf{leaf1, leaf3}, nil)
				tx.EXPECT().Close().Return(nil)
			},
			req:    &getEntryAndProofRequest7,
			errStr: "expected one leaf",
		},
		{
			name: "ok",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				tx.EXPECT().GetLeavesByRange(gomock.Any(), int64(2), int64(1)).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
			},
			req: &getEntryAndProofRequest7,
			wantResp: &trillian.GetEntryAndProofResponse{
				SignedLogRoot: signedRoot1,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					Hashes: [][]byte{
						[]byte("nodehash0"),
						[]byte("nodehash1"),
						[]byte("nodehash2"),
					},
				},
				Leaf: leaf1,
			},
		},
		{
			name: "skew no proof",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
			},
			req: &getEntryAndProofRequest17_11,
			wantResp: &trillian.GetEntryAndProofResponse{
				SignedLogRoot: signedRoot1,
			},
		},
		{
			name: "skew smaller tree",
			setupStorage: func(c *gomock.Controller, s *storage.MockLogStorage) {
				tx := storage.NewMockLogTreeTX(c)
				s.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(tx, nil)
				tx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(signedRoot1, nil)
				tx.EXPECT().GetMerkleNodes(gomock.Any(), nodeIdsInclusionSize7Index2).Return([]tree.Node{
					{ID: nodeIdsInclusionSize7Index2[0], Hash: []byte("nodehash0")},
					{ID: nodeIdsInclusionSize7Index2[1], Hash: []byte("nodehash1")},
					{ID: nodeIdsInclusionSize7Index2[2], Hash: []byte("nodehash2")},
				}, nil)
				tx.EXPECT().GetLeavesByRange(gomock.Any(), int64(2), int64(1)).Return([]*trillian.LogLeaf{leaf1}, nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
				tx.EXPECT().Close().Return(nil)
			},
			req: &getEntryAndProofRequest17_2,
			wantResp: &trillian.GetEntryAndProofResponse{
				SignedLogRoot: signedRoot1,
				Proof: &trillian.Proof{
					LeafIndex: 2,
					Hashes: [][]byte{
						[]byte("nodehash0"),
						[]byte("nodehash1"),
						[]byte("nodehash2"),
					},
				},
				Leaf: leaf1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			fakeStorage := storage.NewMockLogStorage(ctrl)
			tc.setupStorage(ctrl, fakeStorage)
			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: leaf0Request.LogId, numSnapshots: 1, snapErr: tc.snapErr, treeErr: tc.treeErr}),
				LogStorage:   fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)
			resp, err := server.GetEntryAndProof(context.Background(), tc.req)
			if len(tc.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), tc.errStr) {
					t.Errorf("GetEntryAndProof(%v)=%v, %v want nil, err containing: %s", tc.req, resp, err, tc.errStr)
				}
				return
			}

			if err != nil || !proto.Equal(tc.wantResp, resp) {
				t.Errorf("GetEntryAndProof(%v)=%v, %v, want: %v, nil", tc.req, resp, err, tc.wantResp)
			}
		})
	}
}

func TestGetSequencedLeafCountBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	test := newParameterizedTest(ctrl, "GetSequencedLeafCount", readOnly, nopStorage,
		func(t *storage.MockLogTreeTX) {
			t.EXPECT().Close().Return(nil)
		},
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
	fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(mockTX, nil)

	mockTX.EXPECT().GetSequencedLeafCount(gomock.Any()).Return(int64(268), nil)
	mockTX.EXPECT().Commit(gomock.Any()).Return(nil)
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
	req         *trillian.GetConsistencyProofRequest
	errStr      string
	wantHashes  [][]byte
	noSnap      bool
	snapErr     error
	root        *trillian.SignedLogRoot
	noRoot      bool
	rootErr     error
	nodeIDs     []compact.NodeID
	nodes       []tree.Node
	getNodesErr error
	noCommit    bool
	commitErr   error
}

func TestGetConsistencyProof(t *testing.T) {
	tests := []consistProofTest{
		{
			// Storage snapshot fails, should result in an error. Happens before we have a TX so
			// no Close() etc.
			req:      &getConsistencyProofRequest7,
			errStr:   "SnapshotFor",
			snapErr:  errors.New("SnapshotForTree() failed"),
			noRoot:   true,
			noCommit: true,
		},
		{
			// Storage fails to read the log root, should result in an error.
			req:      &getConsistencyProofRequest7,
			errStr:   "LatestSigned",
			rootErr:  errors.New("LatestSignedLogRoot() failed"),
			noCommit: true,
		},
		{
			// Storage fails to unpack the log root, should result in an error.
			req:      &getConsistencyProofRequest7,
			errStr:   "not read current log root",
			root:     corruptLogRoot,
			noCommit: true,
		},
		{
			// Storage fails to get nodes, should result in an error
			req:         &getConsistencyProofRequest7,
			errStr:      "getMerkle",
			nodeIDs:     nodeIdsConsistencySize4ToSize7,
			wantHashes:  [][]byte{[]byte("nodehash")},
			nodes:       []tree.Node{{ID: compact.NewNodeID(2, 1), Hash: []byte("nodehash")}},
			getNodesErr: errors.New("getMerkleNodes() failed"),
			noCommit:    true,
		},
		{
			// Storage fails to commit, should result in an error.
			req:        &getConsistencyProofRequest7,
			errStr:     "commit",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []tree.Node{{ID: compact.NewNodeID(2, 1), Hash: []byte("nodehash")}},
			commitErr:  errors.New("commit() failed"),
		},
		{
			// Storage doesn't return the requested node, should result in an error.
			req:        &getConsistencyProofRequest7,
			errStr:     "expected node {2 1} at",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []tree.Node{{ID: compact.NewNodeID(3, 1), Hash: []byte("nodehash")}},
			noCommit:   true,
		},
		{
			// Storage returns an unexpected extra node, should result in an error.
			req:        &getConsistencyProofRequest7,
			errStr:     "expected 1 nodes",
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []tree.Node{{ID: compact.NewNodeID(2, 1), Hash: []byte("nodehash")}, {ID: compact.NewNodeID(3, 10), Hash: []byte("nodehash2")}},
			noCommit:   true,
		},
		{
			// Ask for a proof from size 4 to 8 but the tree is only size 7. This should succeed but with no proof.
			req:        &getConsistencyProofRequest48,
			wantHashes: nil,
			nodeIDs:    nil,
			noCommit:   true,
		},
		{
			// A normal request which should succeed.
			req:        &getConsistencyProofRequest7,
			wantHashes: [][]byte{[]byte("nodehash")},
			nodeIDs:    nodeIdsConsistencySize4ToSize7,
			nodes:      []tree.Node{{ID: compact.NewNodeID(2, 1), Hash: []byte("nodehash")}},
		},
		{
			// Tests first==second edge case, which should succeed but is an empty proof.
			req:        &getConsistencyProofRequest44,
			wantHashes: [][]byte{},
			nodeIDs:    []compact.NodeID{},
			nodes:      nil,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d:%s", i, test.errStr), func(t *testing.T) {
			fakeStorage := storage.NewMockLogStorage(ctrl)
			mockTX := storage.NewMockLogTreeTX(ctrl)
			if !test.noSnap {
				fakeStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(mockTX, test.snapErr)
			}
			if !test.noRoot {
				root := test.root
				if root == nil {
					root = signedRoot1
				}
				mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(root, test.rootErr)
			}
			if test.nodeIDs != nil {
				mockTX.EXPECT().GetMerkleNodes(gomock.Any(), test.nodeIDs).Return(test.nodes, test.getNodesErr)
			}
			if !test.noCommit {
				mockTX.EXPECT().Commit(gomock.Any()).Return(test.commitErr)
			}
			mockTX.EXPECT().Close().Return(nil)

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: test.req.LogId, numSnapshots: 1}),
				LogStorage:   fakeStorage,
			}
			server := NewTrillianLogRPCServer(registry, fakeTimeSource)
			response, err := server.GetConsistencyProof(context.Background(), test.req)

			if len(test.errStr) > 0 {
				if err == nil || !strings.Contains(err.Error(), test.errStr) {
					t.Errorf("GetConsistencyProof(%+v)=_, %v; want _, err containing %q", test.req, err, test.errStr)
				}
			} else {
				if err != nil {
					t.Errorf("GetConsistencyProof(%+v)=_,%v; want: _,nil", test.req, err)
					return
				}
				if test.wantHashes == nil {
					if response.Proof != nil {
						t.Errorf("GetConsistencyProof(%+v) want nil proof, got %v", test.req, response.Proof)
					}
					return
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
		})
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

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{treeID: test.req.LogId, numSnapshots: 1}),
			}
			logServer := NewTrillianLogRPCServer(registry, fakeTimeSource)

			_, err := logServer.GetInclusionProofByHash(ctx, test.req)
			if s, ok := status.FromError(err); !ok || s.Code() != codes.InvalidArgument {
				t.Errorf("%v: GetInclusionProofByHash() returned err = %v, wantCode = %s", test.desc, err, codes.InvalidArgument)
			}
		})
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
		signFail   bool
		snapErr    error
		treeErr    error
		getRootErr error
		storeErr   error
		wantInit   bool
		slr        *trillian.SignedLogRoot
		wantCode   codes.Code
		wantErrStr string
	}{
		{desc: "snap err", snapErr: errors.New("snap"), wantCode: codes.FailedPrecondition, wantErrStr: "snap"},
		{desc: "tree err", treeErr: errors.New("tree"), wantCode: codes.FailedPrecondition, wantErrStr: "tree"},
		{desc: "root err", getRootErr: errors.New("root"), wantCode: codes.FailedPrecondition, wantErrStr: "root"},
		{desc: "sign fail", getRootErr: storage.ErrTreeNeedsInit, wantInit: true, signFail: true, wantCode: codes.FailedPrecondition, wantErrStr: "Signer()=signature alg"},
		{desc: "store fail", getRootErr: storage.ErrTreeNeedsInit, storeErr: errors.New("store"), wantInit: true, wantCode: codes.FailedPrecondition},
		{desc: "init new log", getRootErr: storage.ErrTreeNeedsInit, wantInit: true, wantCode: codes.OK},
		{desc: "init new preordered log", preordered: true, getRootErr: storage.ErrTreeNeedsInit, wantInit: true, wantCode: codes.OK},
		{desc: "init new log, no err", wantInit: true, wantCode: codes.OK},
		{desc: "init already initialised log", wantInit: false, slr: signedRoot, wantCode: codes.AlreadyExists},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTX := storage.NewMockLogTreeTX(ctrl)
			fakeStorage := &stestonly.FakeLogStorage{TX: mockTX}
			if tc.snapErr == nil && tc.treeErr == nil {
				if tc.getRootErr != nil {
					mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(&trillian.SignedLogRoot{}, tc.getRootErr)
				} else {
					mockTX.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(tc.slr, nil)
				}
				mockTX.EXPECT().IsOpen().AnyTimes().Return(false)
				mockTX.EXPECT().Close().Return(nil)
			}
			if tc.wantInit && !tc.signFail {
				if tc.storeErr == nil {
					mockTX.EXPECT().Commit(gomock.Any()).Return(nil)
				}
				mockTX.EXPECT().StoreSignedLogRoot(gomock.Any(), gomock.Any()).Return(tc.storeErr)
			}

			registry := extension.Registry{
				AdminStorage: fakeAdminStorage(ctrl, storageParams{logID1, tc.preordered, 1, tc.snapErr, tc.treeErr, tc.signFail}),
				LogStorage:   fakeStorage,
			}
			logServer := NewTrillianLogRPCServer(registry, fakeTimeSource)

			c, err := logServer.InitLog(ctx, &trillian.InitLogRequest{LogId: logID1})
			if got, want := status.Code(err), tc.wantCode; got != want {
				t.Errorf("InitLog()=%v,%v, want err code: %v", c, got, want)
			}
			if len(tc.wantErrStr) > 0 && !strings.Contains(err.Error(), tc.wantErrStr) {
				t.Errorf("InitLog()=%v,%v, want err containing: %s", c, err, tc.wantErrStr)
			}
			if tc.wantInit && !tc.signFail && tc.storeErr == nil {
				if err != nil {
					t.Fatalf("InitLog()=%v,%v want err=nil", c, err)
				}
				if c.Created == nil {
					t.Error("InitLog first attempt didn't return a created STH.")
				}
			} else {
				if err == nil {
					t.Errorf("InitLog()=%v,%v want err", c, err)
				}
			}
		})
	}
}

type (
	prepareFakeStorageFunc func(*stestonly.FakeLogStorage)
	prepareMockTXFunc      func(*storage.MockLogTreeTX)
	makeRPCFunc            func(*TrillianLogRPCServer) error
)

type txMode int

func nopTX(_ *storage.MockLogTreeTX)         {}
func nopStorage(_ *stestonly.FakeLogStorage) {}

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
		mockTX.EXPECT().Commit(gomock.Any()).Return(errors.New("bang"))
		mockTX.EXPECT().Close().Return(errors.New("bang"))
		mockTX.EXPECT().IsOpen().AnyTimes().Return(false)
	}

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1, nil, nil, false}),
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
		fakeStorage.EXPECT().SnapshotForTree(ctx, cmpMatcher{tree1}).MaxTimes(1).Return(nil, badLogErr)
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
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1, nil, nil, false}),
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
		logStorage.EXPECT().SnapshotForTree(gomock.Any(), cmpMatcher{tree1}).Return(logTX, errors.New("TX"))
	case readWrite:
		logStorage.EXPECT().ReadWriteTransaction(gomock.Any(), logID, gomock.Any()).Return(errors.New("TX"))
	}

	if p.prepareTX != nil {
		p.prepareTX(logTX)
	}

	registry := extension.Registry{
		AdminStorage: fakeAdminStorage(p.ctrl, storageParams{logID, p.preordered, 1, nil, nil, false}),
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
	snapErr      error
	treeErr      error
	badSigAlg    bool
}

func fakeAdminStorage(ctrl *gomock.Controller, params storageParams) storage.AdminStorage {
	tree := proto.Clone(stestonly.LogTree).(*trillian.Tree)
	if params.preordered {
		tree = proto.Clone(stestonly.PreorderedLogTree).(*trillian.Tree)
	}
	tree.TreeId = params.treeID

	if params.badSigAlg {
		// Force the algorithm to one that can't be signed, which will provoke
		// an error.
		tree.SignatureAlgorithm = sigpb.DigitallySigned_ANONYMOUS
	}

	adminStorage := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)

	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(params.numSnapshots).Return(adminTX, params.snapErr)
	adminTX.EXPECT().GetTree(gomock.Any(), params.treeID).MaxTimes(params.numSnapshots).Return(tree, params.treeErr)
	adminTX.EXPECT().Close().MaxTimes(params.numSnapshots).Return(nil)
	adminTX.EXPECT().Commit().MaxTimes(params.numSnapshots).Return(nil)

	return adminStorage
}

func addTreeID(tree *trillian.Tree, treeID int64) *trillian.Tree {
	newTree := proto.Clone(tree).(*trillian.Tree)
	newTree.TreeId = treeID
	return newTree
}

// newSignerWithFixedSig returns a fake signer that always returns the specified signature.
func newSignerWithFixedSig(sig []byte) crypto.Signer {
	key, err := pem.UnmarshalPublicKey(testonly.DemoPublicKey)
	if err != nil {
		panic(err)
	}
	return testonly.NewSignerWithFixedSig(key, sig)
}

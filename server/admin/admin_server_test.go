// Copyright 2017 Google LLC. All Rights Reserved.
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

package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	ttestonly "github.com/google/trillian/testonly"
)

func TestServer_BeginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	validTree := proto.Clone(testonly.LogTree).(*trillian.Tree)

	tests := []struct {
		desc     string
		fn       func(context.Context, *Server) error
		snapshot bool
	}{
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.ListTrees(ctx, &trillian.ListTreesRequest{})
				return err
			},
			snapshot: true,
		},
		{
			desc: "GetTree",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.GetTree(ctx, &trillian.GetTreeRequest{TreeId: 12345})
				return err
			},
			snapshot: true,
		},
		{
			desc: "CreateTree",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: validTree})
				return err
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		as := storage.NewMockAdminStorage(ctrl)
		if test.snapshot {
			as.EXPECT().Snapshot(gomock.Any()).Return(nil, errors.New("snapshot error"))
		} else {
			as.EXPECT().ReadWriteTransaction(gomock.Any(), gomock.Any()).Return(errors.New("begin error"))
		}

		registry := extension.Registry{
			AdminStorage: as,
		}

		s := &Server{registry: registry}
		if err := test.fn(ctx, s); err == nil {
			t.Errorf("%v: got = %v, want non-nil", test.desc, err)
		}
	}
}

func TestServer_ListTrees(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	activeLog := proto.Clone(testonly.LogTree).(*trillian.Tree)
	frozenLog := proto.Clone(testonly.LogTree).(*trillian.Tree)
	frozenLog.TreeState = trillian.TreeState_FROZEN
	deletedLog := proto.Clone(testonly.LogTree).(*trillian.Tree)

	id := int64(17)
	nowPB := timestamppb.Now()
	for _, tree := range []*trillian.Tree{activeLog, frozenLog, deletedLog} {
		tree.TreeId = id
		tree.CreateTime = proto.Clone(nowPB).(*timestamppb.Timestamp)
		tree.UpdateTime = proto.Clone(nowPB).(*timestamppb.Timestamp)
		id++
		nowPB.Seconds++
	}
	for _, tree := range []*trillian.Tree{deletedLog} {
		tree.Deleted = true
		tree.DeleteTime = proto.Clone(nowPB).(*timestamppb.Timestamp)
		nowPB.Seconds++
	}
	nonDeletedTrees := []*trillian.Tree{activeLog, frozenLog}
	allTrees := []*trillian.Tree{activeLog, frozenLog, deletedLog}

	tests := []struct {
		desc  string
		req   *trillian.ListTreesRequest
		trees []*trillian.Tree
	}{
		{desc: "emptyNonDeleted", req: &trillian.ListTreesRequest{}},
		{desc: "empty", req: &trillian.ListTreesRequest{ShowDeleted: true}},
		{desc: "nonDeleted", req: &trillian.ListTreesRequest{}, trees: nonDeletedTrees},
		{
			desc:  "allTreesDeleted",
			req:   &trillian.ListTreesRequest{ShowDeleted: true},
			trees: allTrees,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			true, /* snapshot */
			true, /* shouldCommit */
			false /* commitErr */)

		tx := setup.snapshotTX
		tx.EXPECT().ListTrees(gomock.Any(), test.req.ShowDeleted).Return(test.trees, nil)

		s := setup.server
		resp, err := s.ListTrees(ctx, test.req)
		if err != nil {
			t.Errorf("%v: ListTrees() returned err = %v", test.desc, err)
			continue
		}
		want := []*trillian.Tree{}
		for _, tree := range test.trees {
			wantTree := proto.Clone(tree).(*trillian.Tree)
			want = append(want, wantTree)
		}
		for i, wantTree := range want {
			if !proto.Equal(resp.Tree[i], wantTree) {
				t.Errorf("%v: post-ListTrees() diff (-got +want):\n%v", test.desc, cmp.Diff(resp.Tree, want))
				break
			}
		}
	}
}

func TestServer_ListTreesErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc      string
		listErr   error
		commitErr bool
	}{
		{desc: "listErr", listErr: errors.New("error listing trees")},
		{desc: "commitErr", commitErr: true},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			true,                /* snapshot */
			test.listErr == nil, /* shouldCommit */
			test.commitErr /* commitErr */)

		tx := setup.snapshotTX
		tx.EXPECT().ListTrees(gomock.Any(), false).Return(nil, test.listErr)

		s := setup.server
		if _, err := s.ListTrees(ctx, &trillian.ListTreesRequest{}); err == nil {
			t.Errorf("%v: ListTrees() returned err = nil, want non-nil", test.desc)
		}
	}
}

func TestServer_GetTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc              string
		getErr, commitErr bool
	}{
		{
			desc: "success",
		},
		{
			desc:   "unknownTree",
			getErr: true,
		},
		{
			desc:      "commitError",
			commitErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			true,         /* snapshot */
			!test.getErr, /* shouldCommit */
			test.commitErr)

		tx := setup.snapshotTX
		s := setup.server

		storedTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
		storedTree.TreeId = 12345
		if test.getErr {
			tx.EXPECT().GetTree(gomock.Any(), storedTree.TreeId).Return(nil, errors.New("GetTree failed"))
		} else {
			tx.EXPECT().GetTree(gomock.Any(), storedTree.TreeId).Return(storedTree, nil)
		}
		wantErr := test.getErr || test.commitErr

		tree, err := s.GetTree(ctx, &trillian.GetTreeRequest{TreeId: storedTree.TreeId})
		if hasErr := err != nil; hasErr != wantErr {
			t.Errorf("%v: GetTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if hasErr {
			continue
		}

		wantTree := proto.Clone(storedTree).(*trillian.Tree)
		if diff := cmp.Diff(tree, wantTree, cmp.Comparer(proto.Equal)); diff != "" {
			t.Errorf("%v: post-GetTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestServer_CreateTree(t *testing.T) {
	validTree := proto.Clone(testonly.LogTree).(*trillian.Tree)

	invalidTree := proto.Clone(validTree).(*trillian.Tree)
	invalidTree.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE

	tests := []struct {
		desc                  string
		req                   *trillian.CreateTreeRequest
		createErr             error
		commitErr, wantCommit bool
		wantErr               string
	}{
		{
			desc:       "validTree",
			req:        &trillian.CreateTreeRequest{Tree: validTree},
			wantCommit: true,
		},
		{
			desc:    "nilTree",
			req:     &trillian.CreateTreeRequest{},
			wantErr: "tree is required",
		},
		{
			desc:      "createErr",
			req:       &trillian.CreateTreeRequest{Tree: invalidTree},
			createErr: errors.New("storage CreateTree failed"),
			wantErr:   "storage CreateTree failed",
		},
		{
			desc:       "commitError",
			req:        &trillian.CreateTreeRequest{Tree: validTree},
			commitErr:  true,
			wantCommit: true,
			wantErr:    "commit error",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			setup := setupAdminServer(ctrl, false /* snapshot */, test.wantCommit, test.commitErr)
			tx := setup.tx
			s := setup.server
			nowPB := timestamppb.Now()

			if test.req.Tree != nil {
				newTree := proto.Clone(test.req.Tree).(*trillian.Tree)
				newTree.TreeId = 12345
				newTree.CreateTime = nowPB
				newTree.UpdateTime = nowPB
				tx.EXPECT().CreateTree(gomock.Any(), gomock.Any()).MaxTimes(1).Return(newTree, test.createErr)
			}

			// Copy test.req so that any changes CreateTree makes don't affect the original, which may be shared between tests.
			reqCopy := proto.Clone(test.req).(*trillian.CreateTreeRequest)
			tree, err := s.CreateTree(ctx, reqCopy)
			switch gotErr := err != nil; {
			case gotErr && !strings.Contains(err.Error(), test.wantErr):
				t.Fatalf("CreateTree() = (_, %q), want (_, %q)", err, test.wantErr)
			case gotErr:
				return
			case test.wantErr != "":
				t.Fatalf("CreateTree() = (_, nil), want (_, %q)", test.wantErr)
			}

			wantTree := proto.Clone(test.req.Tree).(*trillian.Tree)
			wantTree.TreeId = 12345
			wantTree.CreateTime = nowPB
			wantTree.UpdateTime = nowPB
			if diff := cmp.Diff(tree, wantTree, cmp.Comparer(proto.Equal)); diff != "" {
				t.Fatalf("post-CreateTree diff (-got +want):\n%v", diff)
			}
		})
	}
}

func TestServer_CreateTree_AllowedTreeTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc      string
		treeTypes []trillian.TreeType
		req       *trillian.CreateTreeRequest
		wantCode  codes.Code
		wantMsg   string
	}{
		{
			desc:      "preorderedLogOnLogServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_LOG},
			req:       &trillian.CreateTreeRequest{Tree: testonly.PreorderedLogTree},
			wantCode:  codes.InvalidArgument,
			wantMsg:   "tree type PREORDERED_LOG not allowed",
		},
		{
			desc:      "logOnLogServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_LOG},
			req:       &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			wantCode:  codes.OK,
		},
		{
			desc:      "preorderedLogAllowed",
			treeTypes: []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG},
			req:       &trillian.CreateTreeRequest{Tree: testonly.PreorderedLogTree},
			wantCode:  codes.OK,
		},
		// treeTypes = nil is exercised by all other tests.
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false, // snapshot
			test.wantCode == codes.OK,
			false)
		s := setup.server
		tx := setup.tx
		s.allowedTreeTypes = test.treeTypes

		// Storage interactions aren't the focus of this test, so mocks are configured in a rather
		// permissive way.
		tx.EXPECT().CreateTree(gomock.Any(), gomock.Any()).AnyTimes().Return(&trillian.Tree{}, nil)

		_, err := s.CreateTree(ctx, test.req)
		switch s, ok := status.FromError(err); {
		case !ok || s.Code() != test.wantCode:
			t.Errorf("%v: CreateTree() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		case err != nil && !strings.Contains(err.Error(), test.wantMsg):
			t.Errorf("%v: CreateTree() returned err = %q, wantMsg = %q", test.desc, err, test.wantMsg)
		}
	}
}

func TestServer_UpdateTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nowPB := timestamppb.Now()
	existingTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	existingTree.TreeId = 12345
	existingTree.CreateTime = nowPB
	existingTree.UpdateTime = nowPB
	existingTree.MaxRootDuration = durationpb.New(1 * time.Nanosecond)

	// Any valid proto works here, the type doesn't matter for this test.
	settings := ttestonly.MustMarshalAny(t, &emptypb.Empty{})

	// successTree specifies changes in all rw fields
	successTree := &trillian.Tree{
		TreeState:       trillian.TreeState_FROZEN,
		DisplayName:     "Brand New Tree Name",
		Description:     "Brand New Tree Desc",
		StorageSettings: settings,
		MaxRootDuration: durationpb.New(2 * time.Nanosecond),
	}
	successMask := &field_mask.FieldMask{
		Paths: []string{"tree_state", "display_name", "description", "storage_settings", "max_root_duration"},
	}

	successWant := proto.Clone(existingTree).(*trillian.Tree)
	successWant.TreeState = successTree.TreeState
	successWant.DisplayName = successTree.DisplayName
	successWant.Description = successTree.Description
	successWant.StorageSettings = successTree.StorageSettings
	successWant.MaxRootDuration = successTree.MaxRootDuration

	tests := []struct {
		desc                           string
		req                            *trillian.UpdateTreeRequest
		currentTree, wantTree          *trillian.Tree
		updateErr                      error
		commitErr, wantErr, wantCommit bool
	}{
		{
			desc:        "success",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
			currentTree: existingTree,
			wantTree:    successWant,
			wantCommit:  true,
		},
		{
			desc:    "nilTree",
			req:     &trillian.UpdateTreeRequest{},
			wantErr: true,
		},
		{
			desc:        "nilUpdateMask",
			req:         &trillian.UpdateTreeRequest{Tree: successTree},
			currentTree: existingTree,
			wantErr:     true,
		},
		{
			desc:        "emptyUpdateMask",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: &field_mask.FieldMask{}},
			currentTree: existingTree,
			wantErr:     true,
		},
		{
			desc: "readonlyField",
			req: &trillian.UpdateTreeRequest{
				Tree:       successTree,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"tree_id"}},
			},
			currentTree: existingTree,
			wantErr:     true,
		},
		{
			desc:        "updateErr",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
			updateErr:   errors.New("error updating tree"),
			currentTree: existingTree,
			wantErr:     true,
		},
		{
			desc:        "commitErr",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
			currentTree: existingTree,
			commitErr:   true,
			wantErr:     true,
			wantCommit:  true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false, /* snapshot */
			test.wantCommit,
			test.commitErr)

		tx := setup.tx
		s := setup.server

		if test.req.Tree != nil {
			tx.EXPECT().UpdateTree(gomock.Any(), test.req.Tree.TreeId, gomock.Any()).MaxTimes(1).Do(func(ctx context.Context, treeID int64, updateFn func(*trillian.Tree)) {
				// This step should be done by the storage layer, but since we're mocking it we have to trigger it ourselves.
				updateFn(test.currentTree)
			}).Return(test.currentTree, test.updateErr)
		}

		tree, err := s.UpdateTree(ctx, test.req)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: UpdateTree() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if !proto.Equal(tree, test.wantTree) {
			diff := cmp.Diff(tree, test.wantTree)
			t.Errorf("%v: post-UpdateTree diff:\n%v", test.desc, diff)
		}
	}
}

func TestServer_DeleteTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	for i, tree := range []*trillian.Tree{logTree} {
		tree.TreeId = int64(i) + 10
		tree.CreateTime = timestamppb.New(time.Unix(int64(i)*3600, 0))
		tree.UpdateTime = tree.CreateTime
	}

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "logTree", tree: logTree},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false, /* snapshot */
			true,  /* shouldCommit */
			false)
		req := &trillian.DeleteTreeRequest{TreeId: test.tree.TreeId}

		tx := setup.tx
		tx.EXPECT().SoftDeleteTree(gomock.Any(), req.TreeId).Return(test.tree, nil)

		s := setup.server
		got, err := s.DeleteTree(ctx, req)
		if err != nil {
			t.Errorf("%v: DeleteTree() returned err = %v", test.desc, err)
			continue
		}

		want := proto.Clone(test.tree).(*trillian.Tree)
		if !proto.Equal(got, want) {
			diff := cmp.Diff(got, want)
			t.Errorf("%v: post-DeleteTree() diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestServer_DeleteTreeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc      string
		deleteErr error
		commitErr bool
	}{
		{desc: "deleteErr", deleteErr: errors.New("unknown tree")},
		{desc: "commitErr", commitErr: true},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false,
			test.deleteErr == nil,
			test.commitErr)
		req := &trillian.DeleteTreeRequest{TreeId: 10}

		tx := setup.tx
		tx.EXPECT().SoftDeleteTree(gomock.Any(), req.TreeId).Return(&trillian.Tree{}, test.deleteErr)

		s := setup.server
		if _, err := s.DeleteTree(ctx, req); err == nil {
			t.Errorf("%v: DeleteTree() returned err = nil, want non-nil", test.desc)
		}
	}
}

func TestServer_UndeleteTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	activeLog := proto.Clone(testonly.LogTree).(*trillian.Tree)
	frozenLog := proto.Clone(testonly.LogTree).(*trillian.Tree)
	frozenLog.TreeState = trillian.TreeState_FROZEN
	for i, tree := range []*trillian.Tree{activeLog, frozenLog} {
		tree.TreeId = int64(i) + 10
		tree.CreateTime = timestamppb.New(time.Unix(int64(i)*3600, 0))
		tree.UpdateTime = tree.CreateTime
		tree.Deleted = true
		tree.DeleteTime = timestamppb.New(time.Unix(int64(i)*3600+10, 0))
	}

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "activeLog", tree: activeLog},
		{desc: "frozenLog", tree: frozenLog},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false, /* snapshot */
			true,  /* shouldCommit */
			false)
		req := &trillian.UndeleteTreeRequest{TreeId: test.tree.TreeId}

		tx := setup.tx
		tx.EXPECT().UndeleteTree(gomock.Any(), req.TreeId).Return(test.tree, nil)

		s := setup.server
		got, err := s.UndeleteTree(ctx, req)
		if err != nil {
			t.Errorf("%v: UndeleteTree() returned err = %v", test.desc, err)
			continue
		}

		want := proto.Clone(test.tree).(*trillian.Tree)
		if !proto.Equal(got, want) {
			diff := cmp.Diff(got, want)
			t.Errorf("%v: post-UneleteTree() diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestServer_UndeleteTreeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc        string
		undeleteErr error
		commitErr   bool
	}{
		{desc: "undeleteErr", undeleteErr: errors.New("unknown tree")},
		{desc: "commitErr", commitErr: true},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			false,
			test.undeleteErr == nil,
			test.commitErr)
		req := &trillian.UndeleteTreeRequest{TreeId: 10}

		tx := setup.tx
		tx.EXPECT().UndeleteTree(gomock.Any(), req.TreeId).Return(&trillian.Tree{}, test.undeleteErr)

		s := setup.server
		if _, err := s.UndeleteTree(ctx, req); err == nil {
			t.Errorf("%v: UndeleteTree() returned err = nil, want non-nil", test.desc)
		}
	}
}

// adminTestSetup contains an operational Server and required dependencies.
// It's created via setupAdminServer.
type adminTestSetup struct {
	registry   extension.Registry
	as         storage.AdminStorage
	tx         *storage.MockAdminTX
	snapshotTX *storage.MockReadOnlyAdminTX
	server     *Server
}

// setupAdminServer configures mocks according to input parameters.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupAdminServer(ctrl *gomock.Controller, snapshot, shouldCommit, commitErr bool) adminTestSetup {
	as := &testonly.FakeAdminStorage{}

	var snapshotTX *storage.MockReadOnlyAdminTX
	var tx *storage.MockAdminTX
	if snapshot {
		snapshotTX = storage.NewMockReadOnlyAdminTX(ctrl)
		snapshotTX.EXPECT().Close().MaxTimes(1).Return(nil)
		as.ReadOnlyTX = append(as.ReadOnlyTX, snapshotTX)
		if shouldCommit {
			if commitErr {
				snapshotTX.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				snapshotTX.EXPECT().Commit().Return(nil)
			}
		}
	} else {
		tx = storage.NewMockAdminTX(ctrl)
		tx.EXPECT().Close().MaxTimes(1).Return(nil)
		as.TX = append(as.TX, tx)
		if shouldCommit {
			if commitErr {
				tx.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				tx.EXPECT().Commit().Return(nil)
			}
		}
	}

	registry := extension.Registry{AdminStorage: as}

	s := &Server{registry: registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

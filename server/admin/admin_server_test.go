// Copyright 2017 Google Inc. All Rights Reserved.
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
	"errors"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func TestAdminServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, trillian.TrillianAdminServer) error
	}{
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.ListTrees(ctx, &trillian.ListTreesRequest{})
				return err
			},
		},
		{
			desc: "UpdateTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.UpdateTree(ctx, &trillian.UpdateTreeRequest{})
				return err
			},
		},
		{
			desc: "DeleteTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.DeleteTree(ctx, &trillian.DeleteTreeRequest{})
				return err
			},
		},
	}
	ctx := context.Background()
	s := &adminServer{}
	for _, test := range tests {
		if err := test.fn(ctx, s); grpc.Code(err) != codes.Unimplemented {
			t.Errorf("%v: got = %v, want = %s", test.desc, err, codes.Unimplemented)
		}
	}
}

func TestAdminServer_BeginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc     string
		fn       func(context.Context, trillian.TrillianAdminServer) error
		snapshot bool
	}{
		{
			desc: "GetTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.GetTree(ctx, &trillian.GetTreeRequest{TreeId: 12345})
				return err
			},
			snapshot: true,
		},
		{
			desc: "CreateTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: testonly.LogTree})
				return err
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		as := storage.NewMockAdminStorage(ctrl)
		if test.snapshot {
			as.EXPECT().Snapshot(ctx).Return(nil, errors.New("snapshot error"))
		} else {
			as.EXPECT().Begin(ctx).Return(nil, errors.New("begin error"))
		}

		registry := extension.NewMockRegistry(ctrl)
		registry.EXPECT().GetAdminStorage().Return(as)

		s := &adminServer{registry: registry}
		if err := test.fn(ctx, s); err == nil {
			t.Errorf("%v: got = %v, want non-nil", test.desc, err)
		}
	}
}

func TestAdminServer_GetTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	invalidTree := *testonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

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
		setup := setupAdminServer(ctrl, true /* snapshot */, !test.getErr /* shouldCommit */, test.commitErr)
		tx := setup.snapshotTX
		s := setup.server

		req := &trillian.GetTreeRequest{TreeId: 12345}
		wantTree := testonly.LogTree
		if test.getErr {
			tx.EXPECT().GetTree(ctx, req.TreeId).Return(nil, errors.New("GetTree failed"))
		} else {
			tx.EXPECT().GetTree(ctx, req.TreeId).Return(wantTree, nil)
		}
		wantErr := test.getErr || test.commitErr

		tree, err := s.GetTree(ctx, req)
		if hasErr := err != nil; hasErr != wantErr {
			t.Errorf("%v: GetTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if hasErr {
			continue
		}
		if !reflect.DeepEqual(tree, wantTree) {
			t.Errorf("%v: GetTree() = (%v, _), want = (%v, _)", test.desc, tree, wantTree)
		}
	}
}

func TestAdminServer_CreateTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	invalidTree := *testonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	tests := []struct {
		desc                 string
		req                  *trillian.CreateTreeRequest
		createErr, commitErr bool
	}{
		{
			desc: "validTree",
			req:  &trillian.CreateTreeRequest{Tree: testonly.LogTree},
		},
		{
			desc:      "invalidTree",
			req:       &trillian.CreateTreeRequest{Tree: &invalidTree},
			createErr: true,
		},
		{
			desc:      "commitError",
			req:       &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			commitErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(ctrl, false /* snapshot */, !test.createErr /* shouldCommit */, test.commitErr)
		tx := setup.tx
		s := setup.server

		if test.createErr {
			tx.EXPECT().CreateTree(ctx, test.req.Tree).Return(nil, errors.New("CreateTree failed"))
		} else {
			newTree := *test.req.Tree
			tx.EXPECT().CreateTree(ctx, test.req.Tree).Return(&newTree, nil)
		}
		wantErr := test.createErr || test.commitErr

		tree, err := s.CreateTree(ctx, test.req)
		if hasErr := err != nil; hasErr != wantErr {
			t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if hasErr {
			continue
		}
		if !reflect.DeepEqual(tree, test.req.Tree) {
			t.Errorf("%v: CreateTree() = (%v, _), want = (%v, _)", test.desc, tree, test.req.Tree)
		}
	}
}

// adminTestSetup contains an operational adminServer and required dependencies.
// It's created via setupAdminServer.
type adminTestSetup struct {
	registry   *extension.MockRegistry
	as         *storage.MockAdminStorage
	tx         *storage.MockAdminTX
	snapshotTX *storage.MockReadOnlyAdminTX
	server     *adminServer
}

// setupAdminStorage configures storage mocks according to input parameters.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupAdminServer(ctrl *gomock.Controller, snapshot, shouldCommit, commitErr bool) adminTestSetup {
	as := storage.NewMockAdminStorage(ctrl)

	var snapshotTX *storage.MockReadOnlyAdminTX
	var tx *storage.MockAdminTX
	if snapshot {
		snapshotTX = storage.NewMockReadOnlyAdminTX(ctrl)
		as.EXPECT().Snapshot(gomock.Any()).Return(snapshotTX, nil)
		snapshotTX.EXPECT().Close().Return(nil)
		if shouldCommit {
			if commitErr {
				snapshotTX.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				snapshotTX.EXPECT().Commit().Return(nil)
			}
		}
	} else {
		tx = storage.NewMockAdminTX(ctrl)
		as.EXPECT().Begin(gomock.Any()).Return(tx, nil)
		tx.EXPECT().Close().Return(nil)
		if shouldCommit {
			if commitErr {
				tx.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				tx.EXPECT().Commit().Return(nil)
			}
		}
	}

	registry := extension.NewMockRegistry(ctrl)
	registry.EXPECT().GetAdminStorage().Return(as)

	s := &adminServer{registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

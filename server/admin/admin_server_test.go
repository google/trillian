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
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func TestServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, *Server) error
	}{
		{
			desc: "UpdateTree",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.UpdateTree(ctx, &trillian.UpdateTreeRequest{})
				return err
			},
		},
		{
			desc: "DeleteTree",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.DeleteTree(ctx, &trillian.DeleteTreeRequest{})
				return err
			},
		},
	}
	ctx := context.Background()
	s := &Server{}
	for _, test := range tests {
		if err := test.fn(ctx, s); grpc.Code(err) != codes.Unimplemented {
			t.Errorf("%v: got = %v, want = %s", test.desc, err, codes.Unimplemented)
		}
	}
}

func TestServer_BeginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	tests := []struct {
		desc string
		// numTrees is the number of trees in storage. New trees are created as necessary
		// and carried over to following tests.
		numTrees           int
		listErr, commitErr bool
	}{
		{desc: "empty"},
		{desc: "listErr", listErr: true},
		{desc: "commitErr", commitErr: true},
		{desc: "oneTree", numTrees: 1},
		{desc: "threeTrees", numTrees: 3},
	}

	ctx := context.Background()
	nextTreeID := int64(17)
	storedTrees := []*trillian.Tree{}
	for _, test := range tests {
		if l := len(storedTrees); l > test.numTrees {
			t.Fatalf("%v: numTrees = %v, but we already have %v stored trees", test.desc, test.numTrees, l)
		} else {
			for i := l; i < test.numTrees; i++ {
				newTree := *testonly.LogTree
				newTree.TreeId = nextTreeID
				newTree.CreateTimeMillisSinceEpoch = nextTreeID + 1
				newTree.UpdateTimeMillisSinceEpoch = nextTreeID + 2
				nextTreeID++
				storedTrees = append(storedTrees, &newTree)
			}
		}

		setup := setupAdminStorage(
			ctrl,
			true,          /* snapshot */
			!test.listErr, /* shouldCommit */
			test.commitErr)
		tx := setup.snapshotTX
		s := setup.server

		if test.listErr {
			tx.EXPECT().ListTrees(ctx).Return(nil, errors.New("error listing trees"))
		} else {
			// Take a defensive copy, otherwise the server may end up changing our
			// source-of-truth trees.
			trees := copyAndUpdate(storedTrees, func(*trillian.Tree) {})
			tx.EXPECT().ListTrees(ctx).Return(trees, nil)
		}

		resp, err := s.ListTrees(ctx, &trillian.ListTreesRequest{})
		if hasErr, wantErr := err != nil, test.listErr || test.commitErr; hasErr != wantErr {
			t.Errorf("%v: ListTrees() = (_, %q), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if hasErr {
			continue
		}

		want := copyAndUpdate(storedTrees, func(t *trillian.Tree) { t.PrivateKey = nil })
		if diff := pretty.Compare(resp.Tree, want); diff != "" {
			t.Errorf("%v: post-ListTrees diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

// copyAndUpdate makes a deep copy of a slice, allowing for an optional redact function to run on
// every element.
func copyAndUpdate(s []*trillian.Tree, f func(*trillian.Tree)) []*trillian.Tree {
	otherS := make([]*trillian.Tree, 0, len(s))
	for _, t := range s {
		otherT := *t
		f(&otherT)
		otherS = append(otherS, &otherT)
	}
	return otherS
}

func TestServer_GetTree(t *testing.T) {
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
		setup := setupAdminStorage(ctrl, true /* snapshot */, !test.getErr /* shouldCommit */, test.commitErr)
		tx := setup.snapshotTX
		s := setup.server

		storedTree := *testonly.LogTree
		storedTree.TreeId = 12345
		if test.getErr {
			tx.EXPECT().GetTree(ctx, storedTree.TreeId).Return(nil, errors.New("GetTree failed"))
		} else {
			tx.EXPECT().GetTree(ctx, storedTree.TreeId).Return(&storedTree, nil)
		}
		wantErr := test.getErr || test.commitErr

		tree, err := s.GetTree(ctx, &trillian.GetTreeRequest{TreeId: storedTree.TreeId})
		if hasErr := err != nil; hasErr != wantErr {
			t.Errorf("%v: GetTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if hasErr {
			continue
		}

		wantTree := storedTree
		wantTree.PrivateKey = nil // redacted
		if diff := pretty.Compare(tree, &wantTree); diff != "" {
			t.Errorf("%v: post-GetTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestServer_CreateTree(t *testing.T) {
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
		setup := setupAdminStorage(ctrl, false /* snapshot */, !test.createErr /* shouldCommit */, test.commitErr)
		tx := setup.tx
		s := setup.server

		newTree := *test.req.Tree
		newTree.TreeId = 12345
		newTree.CreateTimeMillisSinceEpoch = 1
		newTree.UpdateTimeMillisSinceEpoch = 1
		if test.createErr {
			tx.EXPECT().CreateTree(ctx, test.req.Tree).Return(nil, errors.New("CreateTree failed"))
		} else {
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

		wantTree := newTree
		wantTree.PrivateKey = nil // redacted
		if diff := pretty.Compare(tree, &wantTree); diff != "" {
			t.Errorf("%v: post-CreateTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

// adminTestSetup contains an operational Server and required dependencies.
// It's created via setupAdminServer.
type adminTestSetup struct {
	registry   extension.Registry
	as         *storage.MockAdminStorage
	tx         *storage.MockAdminTX
	snapshotTX *storage.MockReadOnlyAdminTX
	server     *Server
}

// setupAdminStorage configures storage mocks according to input parameters.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupAdminStorage(ctrl *gomock.Controller, snapshot, shouldCommit, commitErr bool) adminTestSetup {
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

	registry := extension.Registry{
		AdminStorage: as,
	}

	s := &Server{registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

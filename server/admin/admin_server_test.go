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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, *Server) error
	}{
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
		err := test.fn(ctx, s)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unimplemented {
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

		// A bit unrealistic, but OK for the purpose of the test.
		sf := keys.NewMockSignerFactory(ctrl)
		sf.EXPECT().NewSigner(gomock.Any(), gomock.Any()).MaxTimes(1).Return(nil, nil)

		registry := extension.Registry{
			AdminStorage:  as,
			SignerFactory: sf,
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

		setup := setupAdminServer(
			ctrl,
			nil,           // key
			true,          // snapshot
			!test.listErr, // shouldCommit
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
		setup := setupAdminServer(ctrl, nil /* key */, true /* snapshot */, !test.getErr /* shouldCommit */, test.commitErr)
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

	// PEM on the testonly trees is ECDSA, so let's use an ECDSA key for tests.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test key: %v", err)
	}

	invalidTree := *testonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	invalidHashAlgo := *testonly.LogTree
	invalidHashAlgo.HashAlgorithm = sigpb.DigitallySigned_NONE

	invalidHashStrategy := *testonly.LogTree
	invalidHashStrategy.HashStrategy = trillian.HashStrategy_UNKNOWN_HASH_STRATEGY

	invalidSignatureAlgo := *testonly.LogTree
	invalidSignatureAlgo.SignatureAlgorithm = sigpb.DigitallySigned_ANONYMOUS

	keySignatureMismatch := *testonly.LogTree
	keySignatureMismatch.SignatureAlgorithm = sigpb.DigitallySigned_RSA

	tests := []struct {
		desc                           string
		req                            *trillian.CreateTreeRequest
		createErr                      error
		commitErr, wantErr, wantCommit bool
	}{
		{
			desc:       "validTree",
			req:        &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			wantCommit: true,
		},
		{
			desc:    "nilTree",
			req:     &trillian.CreateTreeRequest{},
			wantErr: true,
		},
		{
			desc:    "invalidHashAlgo",
			req:     &trillian.CreateTreeRequest{Tree: &invalidHashAlgo},
			wantErr: true,
		},
		{
			desc:    "invalidHashStrategy",
			req:     &trillian.CreateTreeRequest{Tree: &invalidHashStrategy},
			wantErr: true,
		},
		{
			desc:    "invalidSignatureAlgo",
			req:     &trillian.CreateTreeRequest{Tree: &invalidSignatureAlgo},
			wantErr: true,
		},
		{
			desc:    "keySignatureMismatch",
			req:     &trillian.CreateTreeRequest{Tree: &keySignatureMismatch},
			wantErr: true,
		},
		{
			desc:      "createErr",
			req:       &trillian.CreateTreeRequest{Tree: &invalidTree},
			createErr: errors.New("storage CreateTree failed"),
			wantErr:   true,
		},
		{
			desc:       "commitError",
			req:        &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			commitErr:  true,
			wantCommit: true,
			wantErr:    true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(ctrl, key, false /* snapshot */, test.wantCommit, test.commitErr)
		tx := setup.tx
		s := setup.server

		var newTree trillian.Tree
		if test.req.Tree != nil {
			newTree = *test.req.Tree
			newTree.TreeId = 12345
			newTree.CreateTimeMillisSinceEpoch = 1
			newTree.UpdateTimeMillisSinceEpoch = 1
			tx.EXPECT().CreateTree(ctx, test.req.Tree).MaxTimes(1).Return(&newTree, test.createErr)
		}

		tree, err := s.CreateTree(ctx, test.req)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: CreateTree() = (_, %q), wantErr = %v", test.desc, err, test.wantErr)
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

func TestServer_UpdateTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	existingTree := *testonly.LogTree
	existingTree.TreeId = 12345
	existingTree.CreateTimeMillisSinceEpoch = 10
	existingTree.UpdateTimeMillisSinceEpoch = 10

	// Any valid proto works here, the type doesn't matter for this test.
	settings, err := ptypes.MarshalAny(&trillian.PEMKeyFile{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}

	// successTree specifies changes in all rw fields
	successTree := &trillian.Tree{
		TreeState:       trillian.TreeState_FROZEN,
		DisplayName:     "Brand New Tree Name",
		Description:     "Brand New Tree Desc",
		StorageSettings: settings,
	}
	successMask := &field_mask.FieldMask{Paths: []string{"tree_state", "display_name", "description", "storage_settings"}}

	successWant := existingTree
	successWant.TreeState = successTree.TreeState
	successWant.DisplayName = successTree.DisplayName
	successWant.Description = successTree.Description
	successWant.StorageSettings = successTree.StorageSettings
	successWant.PrivateKey = nil // redacted on responses

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
			currentTree: &existingTree,
			wantTree:    &successWant,
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
			currentTree: &existingTree,
			wantErr:     true,
		},
		{
			desc:        "emptyUpdateMask",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: &field_mask.FieldMask{}},
			currentTree: &existingTree,
			wantErr:     true,
		},
		{
			desc: "readonlyField",
			req: &trillian.UpdateTreeRequest{
				Tree:       successTree,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"tree_id"}},
			},
			currentTree: &existingTree,
			wantErr:     true,
		},
		{
			desc:        "updateErr",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
			updateErr:   errors.New("error updating tree"),
			currentTree: &existingTree,
			wantErr:     true,
		},
		{
			desc:        "commitErr",
			req:         &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
			currentTree: &existingTree,
			commitErr:   true,
			wantErr:     true,
			wantCommit:  true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(ctrl, nil /* key */, false /* snapshot */, test.wantCommit, test.commitErr)
		tx := setup.tx
		s := setup.server

		if test.req.Tree != nil {
			tx.EXPECT().UpdateTree(ctx, test.req.Tree.TreeId, gomock.Any()).MaxTimes(1).Return(test.currentTree, test.updateErr)
		}

		tree, err := s.UpdateTree(ctx, test.req)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: UpdateTree() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		// This step should be done by the storage layer, but since we're mocking it we have
		// to trigger it ourselves. Ideally the mock would do it on UpdateTree.
		if err := applyUpdateMask(test.req.Tree, tree, test.req.UpdateMask); err != nil {
			t.Errorf("%v: applyUpdateMask returned err = %v", test.desc, err)
			continue
		}
		if !proto.Equal(tree, test.wantTree) {
			diff := pretty.Compare(tree, test.wantTree)
			t.Errorf("%v: post-UpdateTree diff:\n%v", test.desc, diff)
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

// setupAdminServer configures mocks according to input parameters.
// SignerFactory will be set up to return the received key, if it's non-nil.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupAdminServer(ctrl *gomock.Controller, key crypto.Signer, snapshot, shouldCommit, commitErr bool) adminTestSetup {
	as := storage.NewMockAdminStorage(ctrl)

	var snapshotTX *storage.MockReadOnlyAdminTX
	var tx *storage.MockAdminTX
	if snapshot {
		snapshotTX = storage.NewMockReadOnlyAdminTX(ctrl)
		as.EXPECT().Snapshot(gomock.Any()).MaxTimes(1).Return(snapshotTX, nil)
		snapshotTX.EXPECT().Close().MaxTimes(1).Return(nil)
		if shouldCommit {
			if commitErr {
				snapshotTX.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				snapshotTX.EXPECT().Commit().Return(nil)
			}
		}
	} else {
		tx = storage.NewMockAdminTX(ctrl)
		as.EXPECT().Begin(gomock.Any()).MaxTimes(1).Return(tx, nil)
		tx.EXPECT().Close().MaxTimes(1).Return(nil)
		if shouldCommit {
			if commitErr {
				tx.EXPECT().Commit().Return(errors.New("commit error"))
			} else {
				tx.EXPECT().Commit().Return(nil)
			}
		}
	}

	sf := keys.NewMockSignerFactory(ctrl)
	if key != nil {
		sf.EXPECT().NewSigner(gomock.Any(), gomock.Any()).MaxTimes(1).Return(key, nil)
	}

	registry := extension.Registry{
		AdminStorage:  as,
		SignerFactory: sf,
	}

	s := &Server{registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

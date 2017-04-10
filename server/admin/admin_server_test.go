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
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	terrors "github.com/google/trillian/errors"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly"
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
				_, err := s.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: stestonly.LogTree})
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
			AdminStorage:  as,
			SignerFactory: keys.PEMSignerFactory{},
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

	invalidTree := *stestonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	tests := []struct {
		desc string
		// getErr is the error that will be returned by the mock AdminTX.GetTree() method.
		getErr error
		opts   setupOptions
	}{
		{
			desc: "success",
			opts: setupOptions{
				shouldSnapshot: true,
				shouldCommit:   true,
			},
		},
		{
			desc:   "unknownTree",
			getErr: errors.New("GetTree failed"),
			opts: setupOptions{
				shouldSnapshot: true,
			},
		},
		{
			desc: "commitError",
			opts: setupOptions{
				shouldSnapshot: true,
				shouldCommit:   true,
				commitErr:      errors.New("commit failed"),
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupServer(ctrl, test.opts)
		tx := setup.snapshotTX
		s := setup.server

		storedTree := *stestonly.LogTree
		storedTree.TreeId = 12345

		if test.getErr != nil {
			tx.EXPECT().GetTree(ctx, storedTree.TreeId).Return(nil, test.getErr)
		} else {
			tx.EXPECT().GetTree(ctx, storedTree.TreeId).Return(&storedTree, nil)
		}

		tree, err := s.GetTree(ctx, &trillian.GetTreeRequest{TreeId: storedTree.TreeId})
		if gotErr, wantErr := err != nil, test.opts.wantErr(); gotErr != wantErr {
			t.Errorf("%v: GetTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if gotErr {
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

	invalidTree := *stestonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	nilPrivateKeyTree := *stestonly.LogTree
	nilPrivateKeyTree.PrivateKey = nil

	demoPrivateKey, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		desc string
		req  *trillian.CreateTreeRequest
		// createTree is the tree that is expected to be passed to AdminTX.CreateTree().
		// If nil, no call to AdminTX.CreateTree() is expected.
		createTree *trillian.Tree
		// createErr is the error that will be returned by the mock AdminTX.CreateTree() method.
		createErr error
		opts      setupOptions
	}{
		{
			desc:       "validTree",
			req:        &trillian.CreateTreeRequest{Tree: stestonly.LogTree},
			createTree: stestonly.LogTree,
			opts: setupOptions{
				shouldBeginTx: true,
				shouldCommit:  true,
			},
		},
		{
			desc:       "invalidTree",
			req:        &trillian.CreateTreeRequest{Tree: &invalidTree},
			createTree: &invalidTree,
			createErr:  errors.New("CreateTree failed"),
			opts: setupOptions{
				shouldBeginTx: true,
			},
		},
		{
			desc:       "commitError",
			req:        &trillian.CreateTreeRequest{Tree: stestonly.LogTree},
			createTree: stestonly.LogTree,
			opts: setupOptions{
				shouldBeginTx: true,
				shouldCommit:  true,
				commitErr:     errors.New("commit error"),
			},
		},
		{
			desc: "nilPrivateKeyAndNoKeyGenerator",
			req:  &trillian.CreateTreeRequest{Tree: &nilPrivateKeyTree},
			opts: setupOptions{
				signerFactory: keys.NewMockSignerFactory(ctrl),
			},
		},
		{
			desc:       "nilPrivateKeyAndKeyGenerator",
			req:        &trillian.CreateTreeRequest{Tree: &nilPrivateKeyTree},
			createTree: stestonly.LogTree,
			opts: setupOptions{
				signerFactory: func() keys.Generator {
					keyGen := keys.NewMockGenerator(ctrl)
					gomock.InOrder(
						keyGen.EXPECT().Generate(gomock.Any(), &nilPrivateKeyTree).Return(stestonly.LogTree, nil),
						keyGen.EXPECT().NewSigner(gomock.Any(), stestonly.LogTree).Return(demoPrivateKey, nil),
					)
					return keyGen
				}(),
				shouldBeginTx: true,
				shouldCommit:  true,
			},
		},
		{
			desc: "keyGeneratorError",
			req:  &trillian.CreateTreeRequest{Tree: &nilPrivateKeyTree},
			opts: setupOptions{
				signerFactory: func() keys.Generator {
					keyGen := keys.NewMockGenerator(ctrl)
					keyGen.EXPECT().Generate(gomock.Any(), &nilPrivateKeyTree).Return(nil, terrors.New(terrors.Unknown, "key generation failed"))
					return keyGen
				}(),
			},
		},
		{
			desc: "signerFactoryError",
			req:  &trillian.CreateTreeRequest{Tree: stestonly.LogTree},
			opts: setupOptions{
				signerFactory: func() keys.SignerFactory {
					sf := keys.NewMockSignerFactory(ctrl)
					sf.EXPECT().NewSigner(gomock.Any(), stestonly.LogTree).Return(nil, terrors.New(terrors.InvalidArgument, "invalid key"))
					return sf
				}(),
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		if test.opts.signerFactory == nil {
			signerFactory := keys.NewMockSignerFactory(ctrl)
			signerFactory.EXPECT().NewSigner(gomock.Any(), gomock.Any()).Return(demoPrivateKey, nil)
			test.opts.signerFactory = signerFactory
		}

		setup := setupServer(ctrl, test.opts)
		tx := setup.tx
		s := setup.server

		var newTree *trillian.Tree
		if test.createTree != nil {
			if test.createErr == nil {
				newTree = proto.Clone(test.req.Tree).(*trillian.Tree)
				newTree.TreeId = 12345
				newTree.CreateTimeMillisSinceEpoch = 1
				newTree.UpdateTimeMillisSinceEpoch = 1
			}

			tx.EXPECT().CreateTree(ctx, test.createTree).Return(newTree, test.createErr)
		}

		tree, err := s.CreateTree(ctx, test.req)
		if gotErr, wantErr := err != nil, test.opts.wantErr(); gotErr != wantErr {
			t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, wantErr)
			continue
		} else if gotErr {
			continue
		}

		wantTree := *newTree
		wantTree.PrivateKey = nil // redacted
		if diff := pretty.Compare(tree, &wantTree); diff != "" {
			t.Errorf("%v: post-CreateTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

// testFixture contains an operational Server and required dependencies.
// It's created via setupServer.
type testFixture struct {
	registry   extension.Registry
	as         *storage.MockAdminStorage
	tx         *storage.MockAdminTX
	snapshotTX *storage.MockReadOnlyAdminTX
	server     *Server
}

type setupOptions struct {
	// signerFactory is the keys.SignerFactory to use. If nil, newSignerFactory() will be used.
	signerFactory keys.SignerFactory
	// shouldSnapshot indicates whether AdminStorage.Snapshot() is expected to be called.
	shouldSnapshot bool
	// shouldBeginTx indicates whether AdminStorage.Begin() is expected to be called.
	shouldBeginTx bool
	// shouldCommit indicates whether AdminTX.Commit() is expected to be called.
	shouldCommit bool
	// commitErr is the error that will be returned by the mock AdminTX.Commit() method.
	commitErr error
}

// wantErr returns whether these test options indicate that an error is expected.
func (o *setupOptions) wantErr() bool {
	return !o.shouldCommit || o.commitErr != nil
}

// setupServer configures a Server with mocks according to the input parameters.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupServer(ctrl *gomock.Controller, opts setupOptions) testFixture {
	as := storage.NewMockAdminStorage(ctrl)

	var snapshotTX *storage.MockReadOnlyAdminTX
	if opts.shouldSnapshot {
		snapshotTX = storage.NewMockReadOnlyAdminTX(ctrl)
		prevCall := as.EXPECT().Snapshot(gomock.Any()).Return(snapshotTX, nil)
		if opts.shouldCommit {
			prevCall = snapshotTX.EXPECT().Commit().Return(opts.commitErr).After(prevCall)
		}
		snapshotTX.EXPECT().Close().Return(nil).After(prevCall)
	}

	var tx *storage.MockAdminTX
	if opts.shouldBeginTx {
		tx = storage.NewMockAdminTX(ctrl)
		prevCall := as.EXPECT().Begin(gomock.Any()).Return(tx, nil)
		if opts.shouldCommit {
			prevCall = tx.EXPECT().Commit().Return(opts.commitErr).After(prevCall)
		}
		tx.EXPECT().Close().Return(nil).After(prevCall)
	}

	registry := extension.Registry{
		AdminStorage:  as,
		SignerFactory: opts.signerFactory,
	}

	s := &Server{registry}

	return testFixture{registry, as, tx, snapshotTX, s}
}

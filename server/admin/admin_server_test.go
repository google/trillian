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
	ktestonly "github.com/google/trillian/crypto/keys/testonly"
	terrors "github.com/google/trillian/errors"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func TestAdminServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, *Server) error
	}{
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, s *Server) error {
				_, err := s.ListTrees(ctx, &trillian.ListTreesRequest{})
				return err
			},
		},
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

func TestAdminServer_BeginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc     string
		fn       func(context.Context, *Server) error
		snapshot bool
	}{
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
			AdminStorage:  as,
			SignerFactory: keys.PEMSignerFactory{},
		}

		s := &Server{registry: registry}
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
		desc string
		// getErr is the error that will be returned by the mock AdminTX.GetTree() method.
		getErr error
		opts   testOptions
	}{
		{
			desc: "success",
			opts: testOptions{
				shouldSnapshot: true,
				shouldCommit:   true,
			},
		},
		{
			desc:   "unknownTree",
			getErr: errors.New("GetTree failed"),
			opts: testOptions{
				shouldSnapshot: true,
			},
		},
		{
			desc: "commitError",
			opts: testOptions{
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

		storedTree := *testonly.LogTree
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

func TestAdminServer_CreateTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	invalidTree := *testonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	tests := []struct {
		desc string
		req  *trillian.CreateTreeRequest
		// shouldCreate indicates whether AdminTX.CreateTree() is expected to be called.
		shouldCreate bool
		// createErr is the error that will be returned by the mock AdminTX.CreateTree() method.
		createErr error
		opts      testOptions
	}{
		{
			desc:         "validTree",
			req:          &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			shouldCreate: true,
			opts: testOptions{
				shouldBeginTx: true,
				shouldCommit:  true,
			},
		},
		{
			desc:         "invalidTree",
			req:          &trillian.CreateTreeRequest{Tree: &invalidTree},
			shouldCreate: true,
			createErr:    errors.New("CreateTree failed"),
			opts: testOptions{
				shouldBeginTx: true,
			},
		},
		{
			desc:         "commitError",
			req:          &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			shouldCreate: true,
			opts: testOptions{
				shouldBeginTx: true,
				shouldCommit:  true,
				commitErr:     errors.New("commit error"),
			},
		},
		{
			desc: "non-existent key and SignerFactory does not implement keys.Generator",
			req:  &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			opts: testOptions{
				signerFactory: ktestonly.NewSignerFactoryWithErr(terrors.New(terrors.NotFound, "key not found")),
			},
		},
		{
			desc:         "non-existent key but SignerFactory implements keys.Generator",
			req:          &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			shouldCreate: true,
			opts: testOptions{
				signerFactory: ktestonly.NewKeyGenerator(),
				shouldBeginTx: true,
				shouldCommit:  true,
			},
		},
		{
			desc: "key generation failure",
			req:  &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			opts: testOptions{
				signerFactory: ktestonly.NewKeyGeneratorWithErr(terrors.New(terrors.Unknown, "key generation failed")),
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupServer(ctrl, test.opts)
		tx := setup.tx
		s := setup.server

		var newTree *trillian.Tree
		if test.shouldCreate {
			if test.createErr == nil {
				newTree = proto.Clone(test.req.Tree).(*trillian.Tree)
				newTree.TreeId = 12345
				newTree.CreateTimeMillisSinceEpoch = 1
				newTree.UpdateTimeMillisSinceEpoch = 1
			}

			tx.EXPECT().CreateTree(ctx, test.req.Tree).Return(newTree, test.createErr)
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

type testOptions struct {
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
func (o *testOptions) wantErr() bool {
	return !o.shouldCommit || o.commitErr != nil
}

// setupServer configures a Server with mocks according to the input parameters.
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupServer(ctrl *gomock.Controller, opts testOptions) testFixture {
	as := storage.NewMockAdminStorage(ctrl)

	var snapshotTX *storage.MockReadOnlyAdminTX
	if opts.shouldSnapshot {
		snapshotTX = storage.NewMockReadOnlyAdminTX(ctrl)
		as.EXPECT().Snapshot(gomock.Any()).Return(snapshotTX, nil)
		snapshotTX.EXPECT().Close().Return(nil)
		if opts.shouldCommit {
			snapshotTX.EXPECT().Commit().Return(opts.commitErr)
		}
	}

	var tx *storage.MockAdminTX
	if opts.shouldBeginTx {
		tx = storage.NewMockAdminTX(ctrl)
		as.EXPECT().Begin(gomock.Any()).Return(tx, nil)
		tx.EXPECT().Close().Return(nil)
		if opts.shouldCommit {
			tx.EXPECT().Commit().Return(opts.commitErr)
		}
	}

	signerFactory := opts.signerFactory
	if signerFactory == nil {
		signerFactory = ktestonly.NewSignerFactory()
	}

	registry := extension.Registry{
		AdminStorage:  as,
		SignerFactory: signerFactory,
	}

	s := &Server{registry}

	return testFixture{registry, as, tx, snapshotTX, s}
}

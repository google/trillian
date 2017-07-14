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
	"crypto/x509"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/matchers"
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

	// PEM on the testonly trees is ECDSA, so let's use an ECDSA key for tests.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test key: %v", err)
	}

	// Need to remove the public key, as it won't correspond to the privateKey that was just generated.
	validTree := *testonly.LogTree
	validTree.PublicKey = nil

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
				_, err := s.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: &validTree})
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

		sf := keys.NewMockSignerFactory(ctrl)
		sf.EXPECT().NewSigner(gomock.Any(), gomock.Any()).MaxTimes(1).Return(privateKey, nil)

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
	nowPB, _ := ptypes.TimestampProto(time.Now())
	for _, test := range tests {
		if l := len(storedTrees); l > test.numTrees {
			t.Fatalf("%v: numTrees = %v, but we already have %v stored trees", test.desc, test.numTrees, l)
		} else {
			for i := l; i < test.numTrees; i++ {
				newTree := *testonly.LogTree
				newTree.TreeId = nextTreeID
				newTree.CreateTime = nowPB
				newTree.UpdateTime = nowPB
				nextTreeID++
				storedTrees = append(storedTrees, &newTree)
			}
		}

		setup := setupAdminServer(
			ctrl,
			nil,           // SignerFactory
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
		setup := setupAdminServer(ctrl, nil /* SignerFactory */, true /* snapshot */, !test.getErr /* shouldCommit */, test.commitErr)
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
	var defaultPrivateKey crypto.Signer
	var err error
	defaultPrivateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test key: %v", err)
	}

	defaultPublicKeyDER, err := x509.MarshalPKIXPublicKey(defaultPrivateKey.Public())
	if err != nil {
		t.Fatalf("Error marshaling public key: %v", err)
	}

	// Need to change the public key to correspond with the private key generated above.
	validTree := *testonly.LogTree
	validTree.PublicKey = &keyspb.PublicKey{Der: defaultPublicKeyDER}

	mismatchedPublicKey := *testonly.LogTree

	omittedPublicKey := validTree
	omittedPublicKey.PublicKey = nil

	omittedPrivateKey := validTree
	omittedPrivateKey.PrivateKey = nil

	omittedKeys := omittedPublicKey
	omittedKeys.PrivateKey = nil

	invalidTree := validTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	invalidHashAlgo := validTree
	invalidHashAlgo.HashAlgorithm = sigpb.DigitallySigned_NONE

	invalidHashStrategy := validTree
	invalidHashStrategy.HashStrategy = trillian.HashStrategy_UNKNOWN_HASH_STRATEGY

	invalidSignatureAlgo := validTree
	invalidSignatureAlgo.SignatureAlgorithm = sigpb.DigitallySigned_ANONYMOUS

	keySignatureMismatch := validTree
	keySignatureMismatch.SignatureAlgorithm = sigpb.DigitallySigned_RSA

	tests := []struct {
		desc                           string
		req                            *trillian.CreateTreeRequest
		createErr                      error
		commitErr, wantErr, wantCommit bool
	}{
		{
			desc:       "validTree",
			req:        &trillian.CreateTreeRequest{Tree: &validTree},
			wantCommit: true,
		},
		{
			desc:    "nilTree",
			req:     &trillian.CreateTreeRequest{},
			wantErr: true,
		},
		{
			desc:    "mismatchedPublicKey",
			req:     &trillian.CreateTreeRequest{Tree: &mismatchedPublicKey},
			wantErr: true,
		},
		{
			desc:    "omittedPrivateKey",
			req:     &trillian.CreateTreeRequest{Tree: &omittedPrivateKey},
			wantErr: true,
		},
		{
			desc: "privateKeySpec",
			req: &trillian.CreateTreeRequest{
				Tree: &omittedKeys,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantCommit: true,
		},
		{
			// Tree specifies ECDSA signatures, but key specification provides RSA parameters.
			desc: "privateKeySpecWithMismatchedAlgorithm",
			req: &trillian.CreateTreeRequest{
				Tree: &omittedKeys,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_RsaParams{},
				},
			},
			wantErr: true,
		},
		{
			desc: "privateKeySpecandPrivateKeyProvided",
			req: &trillian.CreateTreeRequest{
				Tree: &validTree,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantErr: true,
		},
		{
			desc: "privateKeySpecAndPublicKeyProvided",
			req: &trillian.CreateTreeRequest{
				Tree: &omittedPrivateKey,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantErr: true,
		},
		{
			desc:       "omittedPublicKey",
			req:        &trillian.CreateTreeRequest{Tree: &omittedPublicKey},
			wantCommit: true,
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
			req:        &trillian.CreateTreeRequest{Tree: &validTree},
			commitErr:  true,
			wantCommit: true,
			wantErr:    true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		sf := keys.NewMockSignerFactory(ctrl)
		privateKey := defaultPrivateKey
		publicKeyDER := defaultPublicKeyDER

		if test.req.GetKeySpec() != nil &&
			test.req.Tree.GetPrivateKey() == nil &&
			test.req.Tree.GetPublicKey() == nil {
			privateKey, err = keys.NewFromSpec(test.req.GetKeySpec())
			if err != nil {
				t.Errorf("%v: failed to generate test private key: %v", test.desc, err)
				continue
			}

			keyDER, err := der.MarshalPrivateKey(privateKey)
			if err != nil {
				t.Errorf("%v: failed to marshal test private key as DER: %v", test.desc, err)
				continue
			}

			keyProto := &keyspb.PrivateKey{Der: keyDER}
			sf.EXPECT().Generate(gomock.Any(), test.req.GetKeySpec()).Return(keyProto, nil)
			sf.EXPECT().NewSigner(gomock.Any(), matchers.ProtoEqual(keyProto)).Return(privateKey, nil)

			publicKeyDER, err = x509.MarshalPKIXPublicKey(privateKey.Public())
			if err != nil {
				t.Errorf("%v: failed to marshal test public key as DER: %v", test.desc, err)
				continue
			}
		} else if test.req.Tree.GetPrivateKey() != nil {
			var keyProto ptypes.DynamicAny
			if err := ptypes.UnmarshalAny(test.req.Tree.GetPrivateKey(), &keyProto); err != nil {
				t.Errorf("%v: failed to unmarshal test.req.Tree.PrivateKey: %v", test.desc, err)
				continue
			}

			sf.EXPECT().NewSigner(gomock.Any(), matchers.ProtoEqual(keyProto.Message)).MaxTimes(1).Return(privateKey, nil)
		}

		setup := setupAdminServer(ctrl, sf, false /* snapshot */, test.wantCommit, test.commitErr)
		tx := setup.tx
		s := setup.server
		nowPB, _ := ptypes.TimestampProto(time.Now())

		if test.req.Tree != nil {
			var newTree trillian.Tree
			tx.EXPECT().CreateTree(ctx, gomock.Any()).MaxTimes(1).Do(func(ctx context.Context, tree *trillian.Tree) {
				newTree = *tree
				newTree.TreeId = 12345
				newTree.CreateTime = nowPB
				newTree.UpdateTime = nowPB
			}).Return(&newTree, test.createErr)
		}

		// Copy test.req so that any changes CreateTree makes don't affect the original, which may be shared between tests.
		reqCopy := proto.Clone(test.req).(*trillian.CreateTreeRequest)
		tree, err := s.CreateTree(ctx, reqCopy)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		wantTree := *test.req.Tree
		wantTree.TreeId = 12345
		wantTree.CreateTime = nowPB
		wantTree.UpdateTime = nowPB
		wantTree.PrivateKey = nil // redacted
		wantTree.PublicKey = &keyspb.PublicKey{Der: publicKeyDER}
		if diff := pretty.Compare(tree, &wantTree); diff != "" {
			t.Errorf("%v: post-CreateTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func marshalECPrivateKeyAsAnyProto(key *ecdsa.PrivateKey) (*any.Any, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	return ptypes.MarshalAny(&keyspb.PrivateKey{
		Der: der,
	})
}

func TestServer_UpdateTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nowPB, _ := ptypes.TimestampProto(time.Now())
	existingTree := *testonly.LogTree
	existingTree.TreeId = 12345
	existingTree.CreateTime = nowPB
	existingTree.UpdateTime = nowPB
	existingTree.MaxRootDuration = ptypes.DurationProto(1 * time.Nanosecond)

	// Any valid proto works here, the type doesn't matter for this test.
	settings, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}

	// successTree specifies changes in all rw fields
	successTree := &trillian.Tree{
		TreeState:       trillian.TreeState_FROZEN,
		DisplayName:     "Brand New Tree Name",
		Description:     "Brand New Tree Desc",
		StorageSettings: settings,
		MaxRootDuration: ptypes.DurationProto(2 * time.Nanosecond),
	}
	successMask := &field_mask.FieldMask{Paths: []string{"tree_state", "display_name", "description", "storage_settings", "max_root_duration"}}

	successWant := existingTree
	successWant.TreeState = successTree.TreeState
	successWant.DisplayName = successTree.DisplayName
	successWant.Description = successTree.Description
	successWant.StorageSettings = successTree.StorageSettings
	successWant.PrivateKey = nil // redacted on responses
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
		setup := setupAdminServer(ctrl, nil /* SignerFactory */, false /* snapshot */, test.wantCommit, test.commitErr)
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
// Storage will be set to use either snapshots or regular TXs via snapshot parameter.
// Whether the snapshot/TX is expected to be committed (and if it should error doing so) is
// controlled via shouldCommit and commitErr parameters.
func setupAdminServer(ctrl *gomock.Controller, sf keys.SignerFactory, snapshot, shouldCommit, commitErr bool) adminTestSetup {
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

	registry := extension.Registry{
		AdminStorage:  as,
		SignerFactory: sf,
	}

	s := &Server{registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

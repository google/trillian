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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ttestonly "github.com/google/trillian/testonly"
)

func TestServer_BeginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// PEM on the testonly trees is ECDSA, so let's use an ECDSA key for tests.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test key: %v", err)
	}

	validTree := proto.Clone(testonly.LogTree).(*trillian.Tree)

	// Need to remove the public key, as it won't correspond to the privateKey that was just generated.
	validTree.PublicKey = nil

	keyProto := &empty.Empty{}
	validTree.PrivateKey = ttestonly.MustMarshalAny(t, keyProto)
	keys.RegisterHandler(fakeKeyProtoHandler(keyProto, privateKey))
	defer keys.UnregisterHandler(keyProto)

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
	activeMap := proto.Clone(testonly.MapTree).(*trillian.Tree)
	deletedMap := proto.Clone(testonly.MapTree).(*trillian.Tree)

	id := int64(17)
	nowPB := ptypes.TimestampNow()
	for _, tree := range []*trillian.Tree{activeLog, frozenLog, deletedLog, activeMap, deletedMap} {
		tree.TreeId = id
		tree.CreateTime = proto.Clone(nowPB).(*timestamp.Timestamp)
		tree.UpdateTime = proto.Clone(nowPB).(*timestamp.Timestamp)
		id++
		nowPB.Seconds++
	}
	for _, tree := range []*trillian.Tree{deletedLog, deletedMap} {
		tree.Deleted = true
		tree.DeleteTime = proto.Clone(nowPB).(*timestamp.Timestamp)
		nowPB.Seconds++
	}
	nonDeletedTrees := []*trillian.Tree{activeLog, frozenLog, activeMap}
	allTrees := []*trillian.Tree{activeLog, frozenLog, deletedLog, activeMap, deletedMap}

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
			nil,  /* keygen */
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
			wantTree.PrivateKey = nil // redacted
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
			nil,                 /* keygen */
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
			nil,          /* keygen */
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
		wantTree.PrivateKey = nil // redacted
		if diff := cmp.Diff(tree, wantTree, cmp.Comparer(proto.Equal)); diff != "" {
			t.Errorf("%v: post-GetTree diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestServer_CreateTree(t *testing.T) {
	// PEM on the testonly trees is ECDSA, so let's use an ECDSA key for tests.
	ecdsaPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test ECDSA key: %v", err)
	}

	rsaPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("Error generating test RSA key: %v", err)
	}

	// Need to change the public key to correspond with the ECDSA private key generated above.
	validTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	// Except in key generation test cases, a keys.ProtoHandler will be registered that
	// returns ecdsaPrivateKey when passed an empty proto.
	wantKeyProto := &empty.Empty{}
	validTree.PrivateKey = ttestonly.MustMarshalAny(t, wantKeyProto)
	validTree.PublicKey = func() *keyspb.PublicKey {
		pb, err := der.ToPublicProto(ecdsaPrivateKey.Public())
		if err != nil {
			t.Fatalf("Error marshaling ECDSA public key: %v", err)
		}
		return pb
	}()

	mismatchedPublicKey := proto.Clone(validTree).(*trillian.Tree)
	mismatchedPublicKey.PublicKey = testonly.LogTree.GetPublicKey()

	omittedPublicKey := proto.Clone(validTree).(*trillian.Tree)
	omittedPublicKey.PublicKey = nil

	omittedPrivateKey := proto.Clone(validTree).(*trillian.Tree)
	omittedPrivateKey.PrivateKey = nil

	omittedKeys := proto.Clone(omittedPublicKey).(*trillian.Tree)
	omittedKeys.PrivateKey = nil

	invalidTree := proto.Clone(validTree).(*trillian.Tree)
	invalidTree.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE

	invalidHashAlgo := proto.Clone(validTree).(*trillian.Tree)
	invalidHashAlgo.HashAlgorithm = sigpb.DigitallySigned_NONE

	invalidHashStrategy := proto.Clone(validTree).(*trillian.Tree)
	invalidHashStrategy.HashStrategy = trillian.HashStrategy_UNKNOWN_HASH_STRATEGY

	invalidSignatureAlgo := proto.Clone(validTree).(*trillian.Tree)
	invalidSignatureAlgo.SignatureAlgorithm = sigpb.DigitallySigned_ANONYMOUS

	keySignatureMismatch := proto.Clone(validTree).(*trillian.Tree)
	keySignatureMismatch.SignatureAlgorithm = sigpb.DigitallySigned_RSA

	tests := []struct {
		desc                  string
		req                   *trillian.CreateTreeRequest
		wantKeyGenerator      bool
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
			desc:    "mismatchedPublicKey",
			req:     &trillian.CreateTreeRequest{Tree: mismatchedPublicKey},
			wantErr: "public and private keys are not a pair",
		},
		{
			desc:    "omittedPrivateKey",
			req:     &trillian.CreateTreeRequest{Tree: omittedPrivateKey},
			wantErr: "private_key or key_spec is required",
		},
		{
			desc: "privateKeySpec",
			req: &trillian.CreateTreeRequest{
				Tree: omittedKeys,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantKeyGenerator: true,
			wantCommit:       true,
		},
		{
			desc: "privateKeySpecButNoKeyGenerator",
			req: &trillian.CreateTreeRequest{
				Tree: omittedKeys,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantErr: "key generation is not enabled",
		},
		{
			// Tree specifies ECDSA signatures, but key specification provides RSA parameters.
			desc: "privateKeySpecWithMismatchedAlgorithm",
			req: &trillian.CreateTreeRequest{
				Tree: omittedKeys,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_RsaParams{},
				},
			},
			wantKeyGenerator: true,
			wantErr:          "signature not supported by signer",
		},
		{
			desc: "privateKeySpecAndPrivateKeyProvided",
			req: &trillian.CreateTreeRequest{
				Tree: validTree,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantKeyGenerator: true,
			wantErr:          "private_key and key_spec fields are mutually exclusive",
		},
		{
			desc: "privateKeySpecAndPublicKeyProvided",
			req: &trillian.CreateTreeRequest{
				Tree: omittedPrivateKey,
				KeySpec: &keyspb.Specification{
					Params: &keyspb.Specification_EcdsaParams{},
				},
			},
			wantKeyGenerator: true,
			wantErr:          "public_key and key_spec fields are mutually exclusive",
		},
		{
			desc:       "omittedPublicKey",
			req:        &trillian.CreateTreeRequest{Tree: omittedPublicKey},
			wantCommit: true,
		},
		{
			desc:    "invalidHashAlgo",
			req:     &trillian.CreateTreeRequest{Tree: invalidHashAlgo},
			wantErr: "unexpected hash algorithm",
		},
		{
			desc:    "invalidHashStrategy",
			req:     &trillian.CreateTreeRequest{Tree: invalidHashStrategy},
			wantErr: "unknown hasher",
		},
		{
			desc:    "invalidSignatureAlgo",
			req:     &trillian.CreateTreeRequest{Tree: invalidSignatureAlgo},
			wantErr: "signature algorithm not supported",
		},
		{
			desc:    "keySignatureMismatch",
			req:     &trillian.CreateTreeRequest{Tree: keySignatureMismatch},
			wantErr: "signature not supported by signer",
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

			var privateKey crypto.Signer = ecdsaPrivateKey
			var keygen keys.ProtoGenerator
			// If KeySpec is set, select the correct type of key to "generate".
			if test.req.GetKeySpec() != nil {
				switch keySpec := test.req.GetKeySpec().GetParams().(type) {
				case *keyspb.Specification_EcdsaParams:
					privateKey = ecdsaPrivateKey
				case *keyspb.Specification_RsaParams:
					privateKey = rsaPrivateKey
				default:
					t.Fatalf("unexpected KeySpec.Params type: %T", keySpec)
				}

				if test.wantKeyGenerator {
					// Setup a fake key generator. If it receives the expected KeySpec, it returns wantKeyProto,
					// which a keys.ProtoHandler will expect to receive later on.
					keygen = fakeKeyProtoGenerator(test.req.GetKeySpec(), wantKeyProto)
				}
			}

			keys.RegisterHandler(fakeKeyProtoHandler(wantKeyProto, privateKey))
			defer keys.UnregisterHandler(wantKeyProto)

			setup := setupAdminServer(ctrl, keygen, false /* snapshot */, test.wantCommit, test.commitErr)
			tx := setup.tx
			s := setup.server
			nowPB := ptypes.TimestampNow()

			if test.req.Tree != nil {
				newTree := proto.Clone(test.req.Tree).(*trillian.Tree)
				newTree.TreeId = 12345
				newTree.CreateTime = nowPB
				newTree.UpdateTime = nowPB
				newTree.PublicKey, err = der.ToPublicProto(privateKey.Public())
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
			wantTree.PrivateKey = nil // redacted
			wantTree.PublicKey, err = der.ToPublicProto(privateKey.Public())
			if err != nil {
				t.Fatalf("failed to marshal test public key as protobuf: %v", err)
			}
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
			desc:      "mapOnLogServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_LOG},
			req:       &trillian.CreateTreeRequest{Tree: testonly.MapTree},
			wantCode:  codes.InvalidArgument,
			wantMsg:   "tree type MAP not allowed",
		},
		{
			desc:      "logOnMapServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_MAP},
			req:       &trillian.CreateTreeRequest{Tree: testonly.LogTree},
			wantCode:  codes.InvalidArgument,
			wantMsg:   "tree type LOG not allowed",
		},
		{
			desc:      "preorderedLogOnLogServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_LOG},
			req:       &trillian.CreateTreeRequest{Tree: testonly.PreorderedLogTree},
			wantCode:  codes.InvalidArgument,
			wantMsg:   "tree type PREORDERED_LOG not allowed",
		},
		{
			desc:      "preorderedLogOnMapServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_MAP},
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
		{
			desc:      "mapOnMapServer",
			treeTypes: []trillian.TreeType{trillian.TreeType_MAP},
			req:       &trillian.CreateTreeRequest{Tree: testonly.MapTree},
			wantCode:  codes.OK,
		},
		// treeTypes = nil is exercised by all other tests.
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			nil,   // keygen
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

	nowPB := ptypes.TimestampNow()
	existingTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	existingTree.TreeId = 12345
	existingTree.CreateTime = nowPB
	existingTree.UpdateTime = nowPB
	existingTree.MaxRootDuration = ptypes.DurationProto(1 * time.Nanosecond)

	// Any valid proto works here, the type doesn't matter for this test.
	settings := ttestonly.MustMarshalAny(t, &empty.Empty{})

	// successTree specifies changes in all rw fields
	successTree := &trillian.Tree{
		TreeState:       trillian.TreeState_FROZEN,
		DisplayName:     "Brand New Tree Name",
		Description:     "Brand New Tree Desc",
		StorageSettings: settings,
		MaxRootDuration: ptypes.DurationProto(2 * time.Nanosecond),
		PrivateKey:      ttestonly.MustMarshalAny(t, &empty.Empty{}),
	}
	successMask := &field_mask.FieldMask{
		Paths: []string{"tree_state", "display_name", "description", "storage_settings", "max_root_duration", "private_key"},
	}

	successWant := proto.Clone(existingTree).(*trillian.Tree)
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
			nil,   /* keygen */
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
	mapTree := proto.Clone(testonly.MapTree).(*trillian.Tree)
	for i, tree := range []*trillian.Tree{logTree, mapTree} {
		tree.TreeId = int64(i) + 10
		tree.CreateTime, _ = ptypes.TimestampProto(time.Unix(int64(i)*3600, 0))
		tree.UpdateTime = tree.CreateTime
	}

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "logTree", tree: logTree},
		{desc: "mapTree", tree: mapTree},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			nil,   /* keygen */
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
		want.PrivateKey = nil // redacted
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
			nil,
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
	activeMap := proto.Clone(testonly.MapTree).(*trillian.Tree)
	for i, tree := range []*trillian.Tree{activeLog, frozenLog, activeMap} {
		tree.TreeId = int64(i) + 10
		tree.CreateTime, _ = ptypes.TimestampProto(time.Unix(int64(i)*3600, 0))
		tree.UpdateTime = tree.CreateTime
		tree.Deleted = true
		tree.DeleteTime, _ = ptypes.TimestampProto(time.Unix(int64(i)*3600+10, 0))
	}

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "activeLog", tree: activeLog},
		{desc: "frozenLog", tree: frozenLog},
		{desc: "activeMap", tree: activeMap},
	}

	ctx := context.Background()
	for _, test := range tests {
		setup := setupAdminServer(
			ctrl,
			nil,   /* keygen */
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
		want.PrivateKey = nil // redacted
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
			nil,
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
func setupAdminServer(ctrl *gomock.Controller, keygen keys.ProtoGenerator, snapshot, shouldCommit, commitErr bool) adminTestSetup {
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

	registry := extension.Registry{
		AdminStorage: as,
		NewKeyProto:  keygen,
	}

	s := &Server{registry: registry}

	return adminTestSetup{registry, as, tx, snapshotTX, s}
}

func fakeKeyProtoHandler(wantKeyProto proto.Message, key crypto.Signer) (proto.Message, keys.ProtoHandler) {
	return wantKeyProto, func(ctx context.Context, gotKeyProto proto.Message) (crypto.Signer, error) {
		if !proto.Equal(gotKeyProto, wantKeyProto) {
			return nil, fmt.Errorf("NewSigner(_, %#v) called, want NewSigner(_, %#v)", gotKeyProto, wantKeyProto)
		}
		return key, nil
	}
}

func fakeKeyProtoGenerator(wantKeySpec *keyspb.Specification, keyProto proto.Message) keys.ProtoGenerator {
	return func(ctx context.Context, gotKeySpec *keyspb.Specification) (proto.Message, error) {
		if !proto.Equal(gotKeySpec, wantKeySpec) {
			return nil, fmt.Errorf("NewKeyProto(_, %#v) called, want NewKeyProto(_, %#v)", gotKeySpec, wantKeySpec)
		}
		return keyProto, nil
	}
}

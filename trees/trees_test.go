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

package trees

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"

	tcrypto "github.com/google/trillian/crypto"
)

func TestFromContext(t *testing.T) {
	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "noTree"},
		{desc: "hasTree", tree: testonly.LogTree},
	}
	for _, test := range tests {
		ctx := NewContext(context.Background(), test.tree)

		tree, ok := FromContext(ctx)
		switch wantOK := test.tree != nil; {
		case ok != wantOK:
			t.Errorf("%v: FromContext(%v) = (_, %v), want = (_, %v)", test.desc, ctx, ok, wantOK)
		case ok && !proto.Equal(tree, test.tree):
			t.Errorf("%v: FromContext(%v) = (%v, nil), want = (%v, nil)", test.desc, ctx, tree, test.tree)
		case !ok && tree != nil:
			t.Errorf("%v: FromContext(%v) = (%v, %v), want = (nil, %v)", test.desc, ctx, tree, ok, wantOK)
		}
	}
}

func TestGetTree(t *testing.T) {
	logTree := *testonly.LogTree
	logTree.TreeId = 1

	mapTree := *testonly.MapTree
	mapTree.TreeId = 2

	frozenTree := *testonly.LogTree
	frozenTree.TreeId = 3
	frozenTree.TreeState = trillian.TreeState_FROZEN

	softDeletedTree := *testonly.LogTree
	softDeletedTree.Deleted = true
	softDeletedTree.DeleteTime = ptypes.TimestampNow()

	tests := []struct {
		desc                           string
		treeID                         int64
		opts                           GetOpts
		ctxTree, storageTree, wantTree *trillian.Tree
		beginErr, getErr, commitErr    error
		wantErr                        bool
	}{
		{
			desc:        "logTree",
			treeID:      logTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &logTree,
			wantTree:    &logTree,
		},
		{
			desc:        "mapTree",
			treeID:      mapTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_MAP},
			storageTree: &mapTree,
			wantTree:    &mapTree,
		},
		{
			desc:        "wrongType1",
			treeID:      logTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_MAP},
			storageTree: &logTree,
			wantErr:     true,
		},
		{
			desc:        "wrongType2",
			treeID:      mapTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &mapTree,
			wantErr:     true,
		},
		{
			desc:        "frozenTree",
			treeID:      frozenTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
			storageTree: &frozenTree,
			wantTree:    &frozenTree,
		},
		{
			desc:        "frozenTreeNotReadonly",
			treeID:      frozenTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &frozenTree,
			wantErr:     true,
		},
		{
			desc:        "softDeleted",
			treeID:      softDeletedTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &softDeletedTree,
			wantErr:     true, // Deleted = true makes the tree "invisible" for most RPCs
		},
		{
			desc:     "treeInCtx",
			treeID:   logTree.TreeId,
			opts:     GetOpts{TreeType: trillian.TreeType_LOG},
			ctxTree:  &logTree,
			wantTree: &logTree,
		},
		{
			desc:        "wrongTreeInCtx",
			treeID:      logTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			ctxTree:     &mapTree,
			storageTree: &logTree,
			wantTree:    &logTree,
		},
		{
			desc:     "beginErr",
			treeID:   logTree.TreeId,
			opts:     GetOpts{TreeType: trillian.TreeType_LOG},
			beginErr: errors.New("begin err"),
			wantErr:  true,
		},
		{
			desc:    "getErr",
			treeID:  logTree.TreeId,
			opts:    GetOpts{TreeType: trillian.TreeType_LOG},
			getErr:  errors.New("get err"),
			wantErr: true,
		},
		{
			desc:      "commitErr",
			treeID:    logTree.TreeId,
			opts:      GetOpts{TreeType: trillian.TreeType_LOG},
			commitErr: errors.New("commit err"),
			wantErr:   true,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, test := range tests {
		ctx := NewContext(context.Background(), test.ctxTree)

		admin := storage.NewMockAdminStorage(ctrl)
		tx := storage.NewMockReadOnlyAdminTX(ctrl)
		admin.EXPECT().Snapshot(ctx).MaxTimes(1).Return(tx, test.beginErr)
		tx.EXPECT().GetTree(ctx, test.treeID).MaxTimes(1).Return(test.storageTree, test.getErr)
		tx.EXPECT().Close().MaxTimes(1).Return(nil)
		tx.EXPECT().Commit().MaxTimes(1).Return(test.commitErr)

		tree, err := GetTree(ctx, admin, test.treeID, test.opts)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTree() = (_, %q), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if !proto.Equal(tree, test.wantTree) {
			diff := pretty.Compare(tree, test.wantTree)
			t.Errorf("%v: post-GetTree diff:\n%v", test.desc, diff)
		}
	}
}

func TestHash(t *testing.T) {
	tests := []struct {
		hashAlgo sigpb.DigitallySigned_HashAlgorithm
		wantHash crypto.Hash
		wantErr  bool
	}{
		{hashAlgo: sigpb.DigitallySigned_NONE, wantErr: true},
		{hashAlgo: sigpb.DigitallySigned_SHA256, wantHash: crypto.SHA256},
	}

	for _, test := range tests {
		tree := *testonly.LogTree
		tree.HashAlgorithm = test.hashAlgo

		hash, err := Hash(&tree)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("Hash(%s) = (_, %q), wantErr = %v", test.hashAlgo, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if hash != test.wantHash {
			t.Errorf("Hash(%s) = (%v, nil), want = (%v, nil)", test.hashAlgo, hash, test.wantHash)
		}
	}
}

func TestSigner(t *testing.T) {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Error generating test ECDSA key: %v", err)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("Error generating test RSA key: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc         string
		sigAlgo      sigpb.DigitallySigned_SignatureAlgorithm
		signer       crypto.Signer
		newSignerErr error
		wantErr      bool
	}{
		{
			desc:    "anonymous",
			sigAlgo: sigpb.DigitallySigned_ANONYMOUS,
			wantErr: true,
		},
		{
			desc:    "ecdsa",
			sigAlgo: sigpb.DigitallySigned_ECDSA,
			signer:  ecdsaKey,
		},
		{
			desc:    "rsa",
			sigAlgo: sigpb.DigitallySigned_RSA,
			signer:  rsaKey,
		},
		{
			desc:    "keyMismatch1",
			sigAlgo: sigpb.DigitallySigned_ECDSA,
			signer:  rsaKey,
			wantErr: true,
		},
		{
			desc:    "keyMismatch2",
			sigAlgo: sigpb.DigitallySigned_RSA,
			signer:  ecdsaKey,
			wantErr: true,
		},
		{
			desc:         "newSignerErr",
			sigAlgo:      sigpb.DigitallySigned_ECDSA,
			newSignerErr: errors.New("NewSigner() error"),
			wantErr:      true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		tree := *testonly.LogTree
		tree.HashAlgorithm = sigpb.DigitallySigned_SHA256
		tree.HashStrategy = trillian.HashStrategy_RFC6962_SHA256
		tree.SignatureAlgorithm = test.sigAlgo

		var wantKeyProto ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(tree.PrivateKey, &wantKeyProto); err != nil {
			t.Errorf("%v: failed to unmarshal tree.PrivateKey: %v", test.desc, err)
		}

		keys.RegisterHandler(wantKeyProto.Message, func(ctx context.Context, gotKeyProto proto.Message) (crypto.Signer, error) {
			if !proto.Equal(gotKeyProto, wantKeyProto.Message) {
				return nil, fmt.Errorf("NewSigner(_, %#v) called, want NewSigner(_, %#v)", gotKeyProto, wantKeyProto.Message)
			}
			return test.signer, test.newSignerErr
		})
		defer keys.UnregisterHandler(wantKeyProto.Message)

		signer, err := Signer(ctx, &tree)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: Signer(_, %s) = (_, %q), wantErr = %v", test.desc, test.sigAlgo, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		want := &tcrypto.Signer{Hash: crypto.SHA256, Signer: test.signer}
		if diff := pretty.Compare(signer, want); diff != "" {
			t.Errorf("%v: post-Signer(_, %s) diff:\n%v", test.desc, test.sigAlgo, diff)
		}
	}
}

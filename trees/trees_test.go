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
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/kylelemons/godebug/pretty"

	tcrypto "github.com/google/trillian/crypto"
	terrors "github.com/google/trillian/errors"
)

var (
	// LogTree is a valid, LOG-type trillian.Tree for tests. Does not
	// contain keys or other things not needed for tree testing.
	LogTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: []byte("dummy_key"),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: []byte("dummy_key"),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}

	// MapTree is a valid, MAP-type trillian.Tree for tests. Does not
	// contain keys or other things not needed for tree testing.
	MapTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_MAP,
		HashStrategy:       trillian.HashStrategy_TEST_MAP_HASHER,
		DisplayName:        "Llamas Map",
		Description:        "Key Transparency map for all your digital llama needs.",
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: []byte("dummy_key"),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: []byte("dummy_key"),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}
)

func TestFromContext(t *testing.T) {
	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "noTree"},
		{desc: "hasTree", tree: LogTree},
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
	logTree := *LogTree
	logTree.TreeId = 1

	mapTree := *MapTree
	mapTree.TreeId = 2

	frozenTree := *LogTree
	frozenTree.TreeId = 3
	frozenTree.TreeState = trillian.TreeState_FROZEN

	softDeletedTree := *LogTree
	softDeletedTree.Deleted = true
	softDeletedTree.DeleteTime = ptypes.TimestampNow()

	tests := []struct {
		desc                           string
		treeID                         int64
		opts                           GetOpts
		ctxTree, storageTree, wantTree *trillian.Tree
		getErr                         error
		wantErr                        bool
		code                           terrors.Code
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
			code:        terrors.InvalidArgument,
		},
		{
			desc:        "wrongType2",
			treeID:      mapTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &mapTree,
			wantErr:     true,
			code:        terrors.InvalidArgument,
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
			code:        terrors.PermissionDenied,
		},
		{
			desc:        "softDeleted",
			treeID:      softDeletedTree.TreeId,
			opts:        GetOpts{TreeType: trillian.TreeType_LOG},
			storageTree: &softDeletedTree,
			wantErr:     true, // Deleted = true makes the tree "invisible" for most RPCs
			code:        terrors.NotFound,
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
			desc:    "getErr",
			treeID:  logTree.TreeId,
			opts:    GetOpts{TreeType: trillian.TreeType_LOG},
			getErr:  errors.New("get err"),
			wantErr: true,
			code:    terrors.Unknown,
		},
	}

	for _, test := range tests {
		ctx := NewContext(context.Background(), test.ctxTree)

		getFunc := func(ctx context.Context, treeID int64) (*trillian.Tree, error) {
			return test.storageTree, test.getErr
		}
		tree, err := GetTree(ctx, getFunc, test.treeID, test.opts)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTree() = (_, %q), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			if terrors.ErrorCode(err) != test.code {
				t.Errorf("%v: GetTree() = (_, %q), got ErrorCode: %v, want: %v", test.desc, err, terrors.ErrorCode(err), test.code)
			}
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
		tree := *LogTree
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
		t.Run(test.desc, func(t *testing.T) {
			tree := *LogTree
			tree.HashAlgorithm = sigpb.DigitallySigned_SHA256
			tree.HashStrategy = trillian.HashStrategy_RFC6962_SHA256
			tree.SignatureAlgorithm = test.sigAlgo

			var wantKeyProto ptypes.DynamicAny
			if err := ptypes.UnmarshalAny(tree.PrivateKey, &wantKeyProto); err != nil {
				t.Fatalf("failed to unmarshal tree.PrivateKey: %v", err)
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
				t.Fatalf("Signer(_, %s) = (_, %q), wantErr = %v", test.sigAlgo, err, test.wantErr)
			} else if hasErr {
				return
			}

			want := &tcrypto.Signer{Hash: crypto.SHA256, Signer: test.signer}
			if diff := pretty.Compare(signer, want); diff != "" {
				t.Fatalf("post-Signer(_, %s) diff:\n%v", test.sigAlgo, diff)
			}
		})
	}
}

func mustMarshalAny(pb proto.Message) *any.Any {
	value, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return value
}

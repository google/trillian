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

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	ktestonly "github.com/google/trillian/crypto/keys/testonly"

	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"
)

const (
	privateKeyPath = "../testdata/log-rpc-server.privkey.pem"
	privateKeyPass = "towel"
	privateKeyPEM  = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,D95ECC664FF4BDEC

Xy3zzHFwlFwjE8L1NCngJAFbu3zFf4IbBOCsz6Fa790utVNdulZncNCl2FMK3U2T
sdoiTW8ymO+qgwcNrqvPVmjFRBtkN0Pn5lgbWhN/aK3TlS9IYJ/EShbMUzjgVzie
S9+/31whWcH/FLeLJx4cBzvhgCtfquwA+s5ojeLYYsk=
-----END EC PRIVATE KEY-----`
	publicKeyPEM = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEywnWicNEQ8bn3GXcGpA+tiU4VL70
Ws9xezgQPrg96YGsFrF6KYG68iqyHDlQ+4FWuKfGKXHn3ooVtB/pfawb5Q==
-----END PUBLIC KEY-----`
)

func TestValidateTreeForCreation(t *testing.T) {
	ctx := context.Background()

	valid1 := newTree()

	valid2 := newTree()
	valid2.TreeType = trillian.TreeType_PREORDERED_LOG

	invalidState1 := newTree()
	invalidState1.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE
	invalidState2 := newTree()
	invalidState2.TreeState = trillian.TreeState_FROZEN

	invalidType := newTree()
	invalidType.TreeType = trillian.TreeType_UNKNOWN_TREE_TYPE

	invalidHashStrategy := newTree()
	invalidHashStrategy.HashStrategy = trillian.HashStrategy_UNKNOWN_HASH_STRATEGY

	invalidHashAlgorithm := newTree()
	invalidHashAlgorithm.HashAlgorithm = sigpb.DigitallySigned_NONE

	invalidSignatureAlgorithm := newTree()
	invalidSignatureAlgorithm.SignatureAlgorithm = sigpb.DigitallySigned_ANONYMOUS

	unsupportedPrivateKey := newTree()
	unsupportedPrivateKey.PrivateKey.TypeUrl = "urn://unknown-type"

	invalidPrivateKey := newTree()
	invalidPrivateKey.PrivateKey.Value = []byte("foobar")

	nilPrivateKey := newTree()
	nilPrivateKey.PrivateKey = nil

	invalidPublicKey := newTree()
	invalidPublicKey.PublicKey.Der = []byte("foobar")

	nilPublicKey := newTree()
	nilPublicKey.PublicKey = nil

	invalidSettings := newTree()
	invalidSettings.StorageSettings = &any.Any{Value: []byte("foobar")}

	// As long as settings is a valid proto, the type doesn't matter for this test.
	settings, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}
	validSettings := newTree()
	validSettings.StorageSettings = settings

	nilRootDuration := newTree()
	nilRootDuration.MaxRootDuration = nil

	invalidRootDuration := newTree()
	invalidRootDuration.MaxRootDuration = durationpb.New(-1 * time.Second)

	deletedTree := newTree()
	deletedTree.Deleted = true

	deleteTimeTree := newTree()
	deleteTimeTree.DeleteTime = timestamppb.Now()

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc: "valid1",
			tree: valid1,
		},
		{
			desc: "valid2",
			tree: valid2,
		},
		{
			desc:    "nilTree",
			tree:    nil,
			wantErr: true,
		},
		{
			desc:    "invalidState1",
			tree:    invalidState1,
			wantErr: true,
		},
		{
			desc:    "invalidState2",
			tree:    invalidState2,
			wantErr: true,
		},
		{
			desc:    "invalidType",
			tree:    invalidType,
			wantErr: true,
		},
		{
			desc:    "invalidHashStrategy",
			tree:    invalidHashStrategy,
			wantErr: true,
		},
		{
			desc:    "invalidHashAlgorithm",
			tree:    invalidHashAlgorithm,
			wantErr: true,
		},
		{
			desc:    "invalidSignatureAlgorithm",
			tree:    invalidSignatureAlgorithm,
			wantErr: true,
		},
		{
			desc:    "unsupportedPrivateKey",
			tree:    unsupportedPrivateKey,
			wantErr: true,
		},
		{
			desc:    "invalidPrivateKey",
			tree:    invalidPrivateKey,
			wantErr: true,
		},
		{
			desc:    "nilPrivateKey",
			tree:    nilPrivateKey,
			wantErr: true,
		},
		{
			desc:    "invalidPublicKey",
			tree:    invalidPublicKey,
			wantErr: true,
		},
		{
			desc:    "nilPublicKey",
			tree:    nilPublicKey,
			wantErr: true,
		},
		{
			desc:    "invalidSettings",
			tree:    invalidSettings,
			wantErr: true,
		},
		{
			desc: "validSettings",
			tree: validSettings,
		},
		{
			desc:    "nilRootDuration",
			tree:    nilRootDuration,
			wantErr: true,
		},
		{
			desc:    "invalidRootDuration",
			tree:    invalidRootDuration,
			wantErr: true,
		},
		{
			desc:    "deletedTree",
			tree:    deletedTree,
			wantErr: true,
		},
		{
			desc:    "deleteTimeTree",
			tree:    deleteTimeTree,
			wantErr: true,
		},
	}
	for _, test := range tests {
		err := ValidateTreeForCreation(ctx, test.tree)
		switch hasErr := err != nil; {
		case hasErr != test.wantErr:
			t.Errorf("%v: ValidateTreeForCreation() = %v, wantErr = %v", test.desc, err, test.wantErr)
		case hasErr && status.Code(err) != codes.InvalidArgument:
			t.Errorf("%v: ValidateTreeForCreation() = %v, wantCode = %v", test.desc, err, codes.InvalidArgument)
		}
	}
}

func TestValidateTreeForUpdate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		desc      string
		treeState trillian.TreeState
		treeType  trillian.TreeType
		updatefn  func(*trillian.Tree)
		wantErr   bool
	}{
		{
			desc: "valid",
			updatefn: func(tree *trillian.Tree) {
				tree.TreeState = trillian.TreeState_FROZEN
				tree.DisplayName = "Frozen Tree"
				tree.Description = "A Frozen Tree"
			},
		},
		{
			desc:     "noop",
			updatefn: func(tree *trillian.Tree) {},
		},
		{
			desc: "validSettings",
			updatefn: func(tree *trillian.Tree) {
				// As long as settings is a valid proto, the type doesn't matter for this test.
				settings, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{})
				if err != nil {
					t.Fatalf("Error marshaling proto: %v", err)
				}
				tree.StorageSettings = settings
			},
		},
		{
			desc: "invalidSettings",
			updatefn: func(tree *trillian.Tree) {
				tree.StorageSettings = &any.Any{Value: []byte("foobar")}
			},
			wantErr: true,
		},
		{
			desc: "validRootDuration",
			updatefn: func(tree *trillian.Tree) {
				tree.MaxRootDuration = durationpb.New(200 * time.Millisecond)
			},
		},
		{
			desc: "invalidRootDuration",
			updatefn: func(tree *trillian.Tree) {
				tree.MaxRootDuration = durationpb.New(-200 * time.Millisecond)
			},
			wantErr: true,
		},
		{
			desc: "differentPrivateKeyProtoButSameKeyMaterial",
			updatefn: func(tree *trillian.Tree) {
				key, err := ptypes.MarshalAny(&keyspb.PrivateKey{
					Der: ktestonly.MustMarshalPrivatePEMToDER(privateKeyPEM, privateKeyPass),
				})
				if err != nil {
					panic(err)
				}
				tree.PrivateKey = key
			},
		},
		{
			desc: "differentPrivateKeyProtoAndDifferentKeyMaterial",
			updatefn: func(tree *trillian.Tree) {
				key, err := ptypes.MarshalAny(&keyspb.PrivateKey{
					Der: ktestonly.MustMarshalPrivatePEMToDER(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass),
				})
				if err != nil {
					panic(err)
				}
				tree.PrivateKey = key
			},
			wantErr: true,
		},
		{
			desc: "unsupportedPrivateKeyProto",
			updatefn: func(tree *trillian.Tree) {
				key, err := ptypes.MarshalAny(&empty.Empty{})
				if err != nil {
					panic(err)
				}
				tree.PrivateKey = key
			},
			wantErr: true,
		},
		{
			desc: "nilPrivateKeyProto",
			updatefn: func(tree *trillian.Tree) {
				tree.PrivateKey = nil
			},
			wantErr: true,
		},
		// Changes on readonly fields
		{
			desc: "TreeId",
			updatefn: func(tree *trillian.Tree) {
				tree.TreeId++
			},
			wantErr: true,
		},
		{
			desc: "TreeType",
			updatefn: func(tree *trillian.Tree) {
				tree.TreeType = trillian.TreeType_PREORDERED_LOG
			},
			wantErr: true,
		},
		{
			desc:     "TreeTypeFromPreorderedLogToLog",
			treeType: trillian.TreeType_PREORDERED_LOG,
			updatefn: func(tree *trillian.Tree) {
				tree.TreeType = trillian.TreeType_LOG
			},
			wantErr: true,
		},
		{
			desc:      "TreeTypeFromFrozenPreorderedLogToLog",
			treeState: trillian.TreeState_FROZEN,
			treeType:  trillian.TreeType_PREORDERED_LOG,
			updatefn: func(tree *trillian.Tree) {
				tree.TreeType = trillian.TreeType_LOG
			},
		},
		{
			desc:      "TreeTypeFromFrozenPreorderedLogToActiveLog",
			treeState: trillian.TreeState_FROZEN,
			treeType:  trillian.TreeType_PREORDERED_LOG,
			updatefn: func(tree *trillian.Tree) {
				tree.TreeState = trillian.TreeState_ACTIVE
				tree.TreeType = trillian.TreeType_LOG
			},
			wantErr: true,
		},
		{
			desc: "HashStrategy",
			updatefn: func(tree *trillian.Tree) {
				tree.HashStrategy = trillian.HashStrategy_UNKNOWN_HASH_STRATEGY
			},
			wantErr: true,
		},
		{
			desc: "HashAlgorithm",
			updatefn: func(tree *trillian.Tree) {
				tree.HashAlgorithm = sigpb.DigitallySigned_NONE
			},
			wantErr: true,
		},
		{
			desc: "SignatureAlgorithm",
			updatefn: func(tree *trillian.Tree) {
				tree.SignatureAlgorithm = sigpb.DigitallySigned_RSA
			},
			wantErr: true,
		},
		{
			desc: "CreateTime",
			updatefn: func(tree *trillian.Tree) {
				tree.CreateTime = timestamppb.New(time.Now())
			},
			wantErr: true,
		},
		{
			desc: "UpdateTime",
			updatefn: func(tree *trillian.Tree) {
				tree.UpdateTime = timestamppb.New(time.Now())
			},
			wantErr: true,
		},
		{
			desc:     "Deleted",
			updatefn: func(tree *trillian.Tree) { tree.Deleted = !tree.Deleted },
			wantErr:  true,
		},
		{
			desc:     "DeleteTime",
			updatefn: func(tree *trillian.Tree) { tree.DeleteTime = timestamppb.Now() },
			wantErr:  true,
		},
	}
	for _, test := range tests {
		tree := newTree()
		if test.treeType != trillian.TreeType_UNKNOWN_TREE_TYPE {
			tree.TreeType = test.treeType
		}
		if test.treeState != trillian.TreeState_UNKNOWN_TREE_STATE {
			tree.TreeState = test.treeState
		}

		baseTree := proto.Clone(tree).(*trillian.Tree)
		test.updatefn(tree)

		err := ValidateTreeForUpdate(ctx, baseTree, tree)
		switch hasErr := err != nil; {
		case hasErr != test.wantErr:
			t.Errorf("%v: ValidateTreeForUpdate() = %v, wantErr = %v", test.desc, err, test.wantErr)
		case hasErr && status.Code(err) != codes.InvalidArgument:
			t.Errorf("%v: ValidateTreeForUpdate() = %v, wantCode = %d", test.desc, err, codes.InvalidArgument)
		}
	}
}

// newTree returns a valid log tree for tests.
func newTree() *trillian.Tree {
	privateKey, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{
		Path:     privateKeyPath,
		Password: privateKeyPass,
	})
	if err != nil {
		panic(err)
	}

	return &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		PrivateKey:         privateKey,
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(publicKeyPEM),
		},
		MaxRootDuration: durationpb.New(1000 * time.Millisecond),
	}
}

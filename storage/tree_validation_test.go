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

package storage

import (
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/errors"
	"github.com/google/trillian/testonly"
)

func TestValidateTreeForCreation(t *testing.T) {
	valid1 := newTree()

	valid2 := newTree()
	valid2.TreeType = trillian.TreeType_MAP

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

	invalidDisplayName := newTree()
	invalidDisplayName.DisplayName = "A Very Long Display Name That Clearly Won't Fit But At Least Mentions Llamas Somewhere"

	invalidDescription := newTree()
	invalidDescription.Description = `
		A Very Long Description That Clearly Won't Fit, Also Mentions Llamas, For Some Reason Has Only Capitalized Words And Keeps Repeating Itself.
		A Very Long Description That Clearly Won't Fit, Also Mentions Llamas, For Some Reason Has Only Capitalized Words And Keeps Repeating Itself.
		`

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
	invalidRootDuration.MaxRootDuration = ptypes.DurationProto(-1 * time.Second)

	deletedTree := newTree()
	deletedTree.Deleted = true

	deleteTimeTree := newTree()
	deleteTimeTree.DeleteTime = ptypes.TimestampNow()

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
			desc:    "invalidDisplayName",
			tree:    invalidDisplayName,
			wantErr: true,
		},
		{
			desc:    "invalidDescription",
			tree:    invalidDescription,
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
		err := ValidateTreeForCreation(test.tree)
		switch hasErr := err != nil; {
		case hasErr != test.wantErr:
			t.Errorf("%v: ValidateTreeForCreation() = %v, wantErr = %v", test.desc, err, test.wantErr)
		case hasErr && errors.ErrorCode(err) != errors.InvalidArgument:
			t.Errorf("%v: ValidateTreeForCreation() = %v, wantCode = %v", test.desc, err, errors.InvalidArgument)
		}
	}
}

func TestValidateTreeForUpdate(t *testing.T) {
	tests := []struct {
		desc     string
		updatefn func(*trillian.Tree)
		wantErr  bool
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
				tree.MaxRootDuration = ptypes.DurationProto(200 * time.Millisecond)
			},
		},
		{
			desc: "invalidRootDuration",
			updatefn: func(tree *trillian.Tree) {
				tree.MaxRootDuration = ptypes.DurationProto(-200 * time.Millisecond)
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
				tree.TreeType = trillian.TreeType_MAP
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
				tree.CreateTime, _ = ptypes.TimestampProto(time.Now())
			},
			wantErr: true,
		},
		{
			desc: "UpdateTime",
			updatefn: func(tree *trillian.Tree) {
				tree.UpdateTime, _ = ptypes.TimestampProto(time.Now())
			},
			wantErr: true,
		},
		{
			desc: "PrivateKey",
			updatefn: func(tree *trillian.Tree) {
				key, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{
					Path: "different.pem",
				})
				if err != nil {
					panic(err)
				}
				tree.PrivateKey = key
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
			updatefn: func(tree *trillian.Tree) { tree.DeleteTime = ptypes.TimestampNow() },
			wantErr:  true,
		},
	}
	for _, test := range tests {
		tree := newTree()
		baseTree := *tree
		test.updatefn(tree)

		err := ValidateTreeForUpdate(&baseTree, tree)
		switch hasErr := err != nil; {
		case hasErr != test.wantErr:
			t.Errorf("%v: ValidateTreeForUpdate() = %v, wantErr = %v", test.desc, err, test.wantErr)
		case hasErr && errors.ErrorCode(err) != errors.InvalidArgument:
			t.Errorf("%v: ValidateTreeForUpdate() = %v, wantCode = %d", test.desc, err, errors.InvalidArgument)
		}
	}
}

// newTree returns a valid tree for tests.
func newTree() *trillian.Tree {
	privateKey, err := ptypes.MarshalAny(&keyspb.PEMKeyFile{
		Path:     "foo.pem",
		Password: "password123",
	})
	if err != nil {
		panic(err)
	}

	publicKeyPEM, _ := pem.Decode([]byte(testonly.DemoPublicKey))
	if publicKeyPEM == nil {
		panic("could not decode public key PEM")
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
		PublicKey:          &keyspb.PublicKey{Der: publicKeyPEM.Bytes},
		MaxRootDuration:    ptypes.DurationProto(1000 * time.Millisecond),
	}
}

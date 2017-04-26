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
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/errors"
)

func TestValidateTreeForCreation(t *testing.T) {
	valid1 := newTree()

	valid2 := newTree()
	valid2.TreeType = trillian.TreeType_MAP

	invalidState1 := newTree()
	invalidState1.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE
	invalidState2 := newTree()
	invalidState2.TreeState = trillian.TreeState_FROZEN
	invalidState3 := newTree()
	invalidState3.TreeState = trillian.TreeState_SOFT_DELETED
	invalidState4 := newTree()
	invalidState4.TreeState = trillian.TreeState_HARD_DELETED

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

	unsupportedKey := newTree()
	unsupportedKey.PrivateKey.TypeUrl = "urn://unknown-type"

	invalidKey := newTree()
	invalidKey.PrivateKey.Value = []byte("foobar")

	nilKey := newTree()
	nilKey.PrivateKey = nil

	invalidSettings := newTree()
	invalidSettings.StorageSettings = &any.Any{Value: []byte("foobar")}

	// As long as settings is a valid proto, the type doesn't matter for this test.
	settings, err := ptypes.MarshalAny(&trillian.PEMKeyFile{})
	if err != nil {
		t.Fatalf("Error marshaling proto: %v", err)
	}
	validSettings := newTree()
	validSettings.StorageSettings = settings

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
			desc:    "invalidState3",
			tree:    invalidState3,
			wantErr: true,
		},
		{
			desc:    "invalidState4",
			tree:    invalidState4,
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
			desc:    "unsupportedKey",
			tree:    unsupportedKey,
			wantErr: true,
		},
		{
			desc:    "invalidKey",
			tree:    invalidKey,
			wantErr: true,
		},
		{
			desc:    "nilKey",
			tree:    nilKey,
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
				settings, err := ptypes.MarshalAny(&trillian.PEMKeyFile{})
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
				tree.CreateTimeMillisSinceEpoch++
			},
			wantErr: true,
		},
		{
			desc: "UpdateTime",
			updatefn: func(tree *trillian.Tree) {
				tree.UpdateTimeMillisSinceEpoch++
			},
			wantErr: true,
		},
		{
			desc: "PrivateKey",
			updatefn: func(tree *trillian.Tree) {
				key, err := ptypes.MarshalAny(&trillian.PEMKeyFile{
					Path: "different.pem",
				})
				if err != nil {
					panic(err)
				}
				tree.PrivateKey = key
			},
			wantErr: true,
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
	privateKey, err := ptypes.MarshalAny(&trillian.PEMKeyFile{
		Path:     "foo.pem",
		Password: "password123",
	})
	if err != nil {
		panic(err)
	}

	return &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC_6962,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		PrivateKey:         privateKey,
	}
}

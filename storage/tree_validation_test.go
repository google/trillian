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
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
)

func TestValidateTreeForCreation(t *testing.T) {
	valid1 := newTree()

	valid2 := newTree()
	valid2.TreeType = trillian.TreeType_MAP
	valid2.DuplicatePolicy = trillian.DuplicatePolicy_DUPLICATES_ALLOWED

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

	invalidDuplicatePolicy := newTree()
	invalidDuplicatePolicy.DuplicatePolicy = trillian.DuplicatePolicy_UNKNOWN_DUPLICATE_POLICY

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

	tests := []struct {
		tree    *trillian.Tree
		wantErr bool
	}{
		{tree: valid1},
		{tree: valid2},
		{tree: invalidState1, wantErr: true},
		{tree: invalidState2, wantErr: true},
		{tree: invalidState3, wantErr: true},
		{tree: invalidState4, wantErr: true},
		{tree: invalidType, wantErr: true},
		{tree: invalidHashStrategy, wantErr: true},
		{tree: invalidHashAlgorithm, wantErr: true},
		{tree: invalidSignatureAlgorithm, wantErr: true},
		{tree: invalidDuplicatePolicy, wantErr: true},
		{tree: invalidDisplayName, wantErr: true},
		{tree: invalidDescription, wantErr: true},
		{tree: unsupportedKey, wantErr: true},
		{tree: invalidKey, wantErr: true},
	}
	for i, test := range tests {
		err := ValidateTreeForCreation(test.tree)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: ValidateTreeForCreation() = %v, wantErr = %v", i, err, test.wantErr)
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
			desc: "DuplicatePolicy",
			updatefn: func(tree *trillian.Tree) {
				tree.DuplicatePolicy = trillian.DuplicatePolicy_DUPLICATES_ALLOWED
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
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: ValidateTreeForUpdate() = %v, wantErr = %v", test.desc, err, test.wantErr)
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
		DuplicatePolicy:    trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		PrivateKey:         privateKey,
	}
}

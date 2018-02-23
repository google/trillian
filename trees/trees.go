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

// Package trees contains utility method for retrieving trees and acquiring objects (hashers,
// signers) associated with them.
package trees

import (
	"crypto"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcrypto "github.com/google/trillian/crypto"
)

type treeKey struct{}

// NewContext returns a ctx with the given tree.
func NewContext(ctx context.Context, tree *trillian.Tree) context.Context {
	return context.WithValue(ctx, treeKey{}, tree)
}

// FromContext returns the tree within ctx if present, together with an indication of whether a
// tree was present.
func FromContext(ctx context.Context) (*trillian.Tree, bool) {
	tree, ok := ctx.Value(treeKey{}).(*trillian.Tree)
	return tree, ok && tree != nil
}

func validate(o GetOpts, tree *trillian.Tree) error {
	// TODO(Martin2112): Enforce access type here - must be valid for tree
	// state and not set to Unknown.
	switch {
	case len(o.TreeTypes) > 0 && !o.TreeTypes[tree.TreeType]:
		return status.Errorf(codes.InvalidArgument, "operation not allowed for %s-type trees (wanted one of %v)", tree.TreeType, o.TreeTypes)
	case tree.TreeState == trillian.TreeState_FROZEN && !o.Readonly:
		return status.Errorf(codes.PermissionDenied, "operation not allowed on %s trees", tree.TreeState)
	}

	return nil
}

// GetTree returns the specified tree, either from the ctx (if present) or read from storage.
// The tree will be validated according to GetOpts before returned. Tree state is also considered
// (for example, deleted tree will return NotFound errors).
func GetTree(ctx context.Context, s storage.AdminStorage, treeID int64, opts GetOpts) (*trillian.Tree, error) {
	// TODO(codingllama): Record stats of ctx hits/misses, so we can assess whether RPCs work
	// as intended.
	tree, ok := FromContext(ctx)
	if !ok || tree.TreeId != treeID {
		var err error
		tree, err = storage.GetTree(ctx, s, treeID)
		if err != nil {
			return nil, err
		}
	}

	if err := validate(opts, tree); err != nil {
		return nil, err
	}
	if tree.Deleted {
		return nil, status.Errorf(codes.NotFound, "tree %v not found", tree.TreeId)
	}

	return tree, nil
}

// Hash returns the crypto.Hash configured by the tree.
func Hash(tree *trillian.Tree) (crypto.Hash, error) {
	switch tree.HashAlgorithm {
	case sigpb.DigitallySigned_SHA256:
		return crypto.SHA256, nil
	}
	// There's no nil-like value for crypto.Hash, something has to be returned.
	return crypto.SHA256, fmt.Errorf("unexpected hash algorithm: %s", tree.HashAlgorithm)
}

// Signer returns a Trillian crypto.Signer configured by the tree.
func Signer(ctx context.Context, tree *trillian.Tree) (*tcrypto.Signer, error) {
	if tree.SignatureAlgorithm == sigpb.DigitallySigned_ANONYMOUS {
		return nil, fmt.Errorf("signature algorithm not supported: %s", tree.SignatureAlgorithm)
	}

	hash, err := Hash(tree)
	if err != nil {
		return nil, err
	}

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.PrivateKey, &keyProto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree.PrivateKey: %v", err)
	}

	signer, err := keys.NewSigner(ctx, keyProto.Message)
	if err != nil {
		return nil, err
	}

	if tcrypto.SignatureAlgorithm(signer.Public()) != tree.SignatureAlgorithm {
		return nil, fmt.Errorf("%s signature not supported by signer of type %T", tree.SignatureAlgorithm, signer)
	}

	return &tcrypto.Signer{Hash: hash, Signer: signer}, nil
}

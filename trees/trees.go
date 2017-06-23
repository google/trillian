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
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"

	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/errors"
	"github.com/google/trillian/storage"
	"golang.org/x/net/context"
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

// GetOpts contains validation options for GetTree.
type GetOpts struct {
	// TreeType is the expected type of the tree. Use trillian.TreeType_UNKNOWN_TREE_TYPE to
	// allow any type.
	TreeType trillian.TreeType
	// Readonly is whether the tree will be used for read-only purposes.
	Readonly bool
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
		tree, err = getTreeFromStorage(ctx, s, treeID)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case opts.TreeType != trillian.TreeType_UNKNOWN_TREE_TYPE && tree.TreeType != opts.TreeType:
		return nil, errors.Errorf(errors.InvalidArgument, "operation not allowed for %s-type trees (wanted %s-type)", tree.TreeType, opts.TreeType)
	case tree.TreeState == trillian.TreeState_FROZEN && !opts.Readonly:
		return nil, errors.Errorf(errors.FailedPrecondition, "operation not allowed on %s trees", tree.TreeState)
	case tree.TreeState == trillian.TreeState_SOFT_DELETED || tree.TreeState == trillian.TreeState_HARD_DELETED:
		return nil, errors.Errorf(errors.NotFound, "deleted tree: %v", tree.TreeId)
	}

	return tree, nil
}

func getTreeFromStorage(ctx context.Context, s storage.AdminStorage, treeID int64) (*trillian.Tree, error) {
	tx, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	tree, err := tx.GetTree(ctx, treeID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
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
func Signer(ctx context.Context, sf keys.SignerFactory, tree *trillian.Tree) (*tcrypto.Signer, error) {
	if tree.SignatureAlgorithm == sigpb.DigitallySigned_ANONYMOUS {
		return nil, fmt.Errorf("signature algorithm not supported: %s", tree.SignatureAlgorithm)
	}

	hash, err := Hash(tree)
	if err != nil {
		return nil, err
	}

	signer, err := sf.NewSigner(ctx, tree.PrivateKey)
	if err != nil {
		return nil, err
	}

	var ok bool
	switch signer.(type) {
	case *ecdsa.PrivateKey:
		ok = tree.SignatureAlgorithm == sigpb.DigitallySigned_ECDSA
	case *rsa.PrivateKey:
		ok = tree.SignatureAlgorithm == sigpb.DigitallySigned_RSA
	default:
		// TODO(codingllama): Make SignatureAlgorithm / key matching part of the SignerFactory contract?
		// We don't know about custom signers, so let it pass
		ok = true
	}
	if !ok {
		return nil, fmt.Errorf("%s signature not supported by key of type %T", tree.SignatureAlgorithm, signer)
	}
	return &tcrypto.Signer{Hash: hash, Signer: signer}, nil
}
